package core

import (
	"sync"
)

// BatchSyncTokenInfoPool manages a pool of BatchSyncTokenInfo objects
type BatchSyncTokenInfoPool struct {
	pool sync.Pool
}

// NewBatchSyncTokenInfoPool creates a new batch sync token info pool
func NewBatchSyncTokenInfoPool() *BatchSyncTokenInfoPool {
	return &BatchSyncTokenInfoPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &BatchSyncTokenInfo{}
			},
		},
	}
}

// Get retrieves a BatchSyncTokenInfo from the pool
func (p *BatchSyncTokenInfoPool) Get() *BatchSyncTokenInfo {
	return p.pool.Get().(*BatchSyncTokenInfo)
}

// Put returns a BatchSyncTokenInfo to the pool after resetting it
func (p *BatchSyncTokenInfoPool) Put(bsti *BatchSyncTokenInfo) {
	// Reset ALL fields to prevent state leakage between transactions
	bsti.Token = ""
	bsti.BlockID = ""
	bsti.TokenType = 0

	p.pool.Put(bsti)
}
