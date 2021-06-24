package tangle

import (
	"github.com/cockroachdb/errors"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/objectstorage"
)

type EligibilityEvents = struct {
	// MessageEligible is triggered when messege eligible flag is set to true
	MessageEligible *events.Event
	Error           *events.Event
}

type EligibilityManager = struct {
	Events *EligibilityEvents

	tangle *Tangle
}

func (e *EligibilityManager) checkEligibility(messageID MessageID) error {
	cachedMsg := e.tangle.Storage.Message(messageID)
	defer cachedMsg.Release()

	message := cachedMsg.Unwrap()
	payloadType := message.Payload().Type()
	if payloadType != ledgerstate.TransactionType {
		setEligible(message)
		return nil
	}

	tx := message.Payload().(*ledgerstate.Transaction)
	pendingDependencies, err := obtainPendingDependencies(tx, e.tangle.LedgerState.UTXODAG)
	if err != nil {
		return errors.Errorf("failed to get pending transaction's dependencies: %w", err)
	}
	if len(pendingDependencies) == 0 {
		setEligible(message)
		return nil
	}

	for _, dependencyTxID := range pendingDependencies {
		// TODO
	}
	return nil
}

// obtainPendingDependencies return all transaction ids that are still pending confirmation that tx depends upon
func obtainPendingDependencies(tx *ledgerstate.Transaction, utxoDAG *ledgerstate.UTXODAG) ([]*ledgerstate.TransactionID, error) {
	pendingDependencies := make([]*ledgerstate.TransactionID, 0)
	for _, input := range tx.Essence().Inputs() {
		outputID := input.(*ledgerstate.UTXOInput).ReferencedOutputID()
		txID := outputID.TransactionID()
		state, err := utxoDAG.InclusionState(txID)
		if err != nil {
			return nil, errors.Errorf("failed to get inclusion state: %w", err)
		}
		if state != ledgerstate.Confirmed {
			pendingDependencies = append(pendingDependencies, &txID)
		}
	}
	return pendingDependencies, nil
}

func NewEligibilityManager(tangle *Tangle) {
	eligibilityManager = &EligibilityManager{
		Events: &EligibilityEvents{
			MessageEligible: events.NewEvent(),
			Error:           events.NewEvent(events.ErrorCaller),
		},

		tangle: tangle,
	}
}

// Setup sets up the behavior of the component by making it attach to the relevant events of other components.
func (e *EligibilityManager) Setup() {
	e.tangle.Solidifier.Events.MessageSolid.Attach(events.NewClosure(func(messageID MessageID) {
		if err := e.checkEligibility(messageID); err != nil {
			e.Events.Error.Trigger(errors.Errorf("failed to check eligibility of message %s. %w", messageID, err))
		}
	}))
}

func UnconfirmedTxDependenciesStorage(key, data []byte) (result objectstorage.StorableObject, err error) {
	// TODO
	return nil, nil
}

func Setup() {
}

//var unconfirmedTxDependencies map[TransactionID]TransactionIDs
//func OnMessageSolid(message *Message) {
//	if !message.ContainsTX() {
//		message.SetEligible(true)
//		return
//	}
//	if pendingDependencies := obtainPendingDependencies(message); len(pendingDependencies) == 0 {
//		message.SetEligible(true)
//		return
//	}
//	for dependencyTransactionID := range pendingDependencies {
//		if transactionIDs, exists := unconfirmedTxDependencies[dependencyTransactionID]; !exists {
//			unconfirmedTxDependencies[dependencyTransactionID] = NewTransactionIDs()
//		}
//		unconfirmedTxDependencies[dependencyTransactionID].Add(message.payload.(*Transaction).ID())
//	}
//}
//func OnTransactionConfirmed(tx *Transaction) {
//	for dependentTransaction := range unconfirmedTxDependencies[tx.ID()] {
//		for attachment := range Attachments(dependentTransaction) {
//			attachment.SetEligible(true)
//		}
//	}
//}
