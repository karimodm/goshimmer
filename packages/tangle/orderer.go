package tangle

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/events"
)

const (
	// allowedFutureBooking defines the duration in which messages ahead of the TangleTime can be forwarded to the scheduler.
	allowedFutureBooking = 30 * time.Minute
)

// Orderer is a Tangle component that makes sure that no messages too far ahead of the TangleTime are booked.
// This is necessary to basically replay the tangle data structure as it was constructed during syncing to avoid
// distortions in perceptions of the approval weight.
type Orderer struct {
	Events *OrdererEvents

	tangle         *Tangle
	shutdownSignal chan struct{}
	shutdownWG     sync.WaitGroup
	shutdownOnce   sync.Once

	bookedMessageChan chan MessageID
	inbox             chan MessageID
	parentsMap        map[MessageID][]MessageID
}

// NewOrderer is the constructor for Orderer.
func NewOrderer(tangle *Tangle) (orderer *Orderer) {
	orderer = &Orderer{
		Events: &OrdererEvents{
			MessageOrdered: events.NewEvent(MessageIDCaller),
		},
		tangle:            tangle,
		shutdownSignal:    make(chan struct{}),
		bookedMessageChan: make(chan MessageID),
		inbox:             make(chan MessageID, 1024),
		parentsMap:        make(map[MessageID][]MessageID),
	}
	orderer.run()

	return
}

func (o *Orderer) onMessageBooked(messageID MessageID) {
	//fmt.Println(messageID)
	o.bookedMessageChan <- messageID
}

func (o *Orderer) parentsToBook(messageID MessageID) (parents MessageIDs) {
	o.tangle.Storage.Message(messageID).Consume(func(message *Message) {
		message.ForEachParent(func(parent Parent) {
			o.tangle.Storage.MessageMetadata(parent.ID).Consume(func(messageMetadata *MessageMetadata) {
				if !messageMetadata.IsBooked() {
					parents = append(parents, parent.ID)
				}
			})
		})
	})

	return parents
}

func (o *Orderer) tryToSchedule(messageID MessageID) (parentsToBook []MessageID) {
	parentsToBook = o.parentsToBook(messageID)
	if len(parentsToBook) > 0 {
		return
	}

	// all parents are booked
	o.Events.MessageOrdered.Trigger(messageID)

	return
}

// Setup sets up the behavior of the component by making it attach to the relevant events of other components.
func (o *Orderer) Setup() {
	// o.tangle.Solidifier.Events.MessageSolid.Attach(events.NewClosure(o.Order))
	o.tangle.Booker.Events.MessageBooked.Attach(events.NewClosure(o.onMessageBooked))
}

// Shutdown shuts down the Orderer and persists its state.
func (o *Orderer) Shutdown() {
	o.shutdownOnce.Do(func() {
		close(o.shutdownSignal)
	})

	o.shutdownWG.Wait()
}

// run runs the background thread that listens to TangleTime updates (through a channel) and then schedules messages
// as the TangleTime advances forward.
func (o *Orderer) run() {
	o.shutdownWG.Add(1)
	go func() {
		defer o.shutdownWG.Done()

		for {
			select {
			case bookedMessage := <-o.bookedMessageChan:
				if _, exists := o.parentsMap[bookedMessage]; !exists {
					continue
				}

				for _, childID := range o.parentsMap[bookedMessage] {
					o.tryToSchedule(childID)
				}
				delete(o.parentsMap, bookedMessage)
			case messageID := <-o.inbox:
				parentsToBook := o.tryToSchedule(messageID)

				for _, parent := range parentsToBook {
					if _, exists := o.parentsMap[parent]; !exists {
						o.parentsMap[parent] = make([]MessageID, 0)
					}
					o.parentsMap[parent] = append(o.parentsMap[parent], messageID)
				}
			case <-o.shutdownSignal:
				if len(o.inbox) == 0 {
					return
				}
			}
		}
	}()
}

// region OrdererEvents ////////////////////////////////////////////////////////////////////////////////////////////////

// OrdererEvents represents events happening in the Orderer.
type OrdererEvents struct {
	// MessageOrdered is triggered when a message is ordered and thus ready to be scheduled.
	MessageOrdered *events.Event
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////