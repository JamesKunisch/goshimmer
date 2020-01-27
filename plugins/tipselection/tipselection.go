package tipselection

import (
	"math/rand"
	"sync"

	"github.com/iotaledger/goshimmer/packages/model/meta_transaction"
	"github.com/iotaledger/iota.go/trinary"
)

var (
	tipSet = make(map[trinary.Hash]struct{})
	mutex  sync.RWMutex
)

func GetRandomTip() trinary.Trytes {
	mutex.RLock()
	defer mutex.RUnlock()

	if len(tipSet) == 0 {
		return meta_transaction.BRANCH_NULL_HASH
	}
	i := rand.Intn(len(tipSet))
	for k := range tipSet {
		if i == 0 {
			return k
		}
		i--
	}
	panic("unreachable")
}

func GetTipsCount() int {
	mutex.RLock()
	defer mutex.RUnlock()

	return len(tipSet)
}
