package gossip

import (
    "github.com/iotaledger/goshimmer/packages/filter"
    "github.com/iotaledger/goshimmer/packages/transaction"
)

// region public api ///////////////////////////////////////////////////////////////////////////////////////////////////

func ProcessReceivedTransactionData(transactionData []byte) {
    if transactionFilter.Add(transactionData) {
        Events.ReceiveTransaction.Trigger(transaction.FromBytes(transactionData))
    }
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region constants and variables //////////////////////////////////////////////////////////////////////////////////////

var transactionFilter = filter.NewByteArrayFilter(TRANSACTION_FILTER_SIZE)

const (
    TRANSACTION_FILTER_SIZE = 5000
)

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////