package tipselection

import (
	"github.com/iotaledger/goshimmer/packages/model/value_transaction"
	"github.com/iotaledger/goshimmer/plugins/tangle"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/node"
)

var PLUGIN = node.NewPlugin("Tipselection", node.Enabled, configure, run)

func configure(*node.Plugin) {
	tangle.Events.TransactionSolid.Attach(events.NewClosure(func(transaction *value_transaction.ValueTransaction) {
		mutex.Lock()
		defer mutex.Unlock()

		delete(tipSet, transaction.GetBranchTransactionHash())
		delete(tipSet, transaction.GetTrunkTransactionHash())
		tipSet[transaction.GetHash()] = struct{}{}
	}))
}

func run(*node.Plugin) {
}
