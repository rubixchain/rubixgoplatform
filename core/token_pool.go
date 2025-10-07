package core

import (
	"sync"
	
	"github.com/rubixchain/rubixgoplatform/contract"
)

// TokenInfoPool manages a pool of TokenInfo objects to reduce GC pressure
type TokenInfoPool struct {
	pool sync.Pool
}

// NewTokenInfoPool creates a new token info pool
func NewTokenInfoPool() *TokenInfoPool {
	return &TokenInfoPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &contract.TokenInfo{}
			},
		},
	}
}

// Get retrieves a TokenInfo from the pool
func (p *TokenInfoPool) Get() *contract.TokenInfo {
	return p.pool.Get().(*contract.TokenInfo)
}

// Put returns a TokenInfo to the pool after resetting it
func (p *TokenInfoPool) Put(ti *contract.TokenInfo) {
	// Reset ALL fields to prevent state leakage between transactions
	ti.Token = ""
	ti.TokenType = 0
	ti.TokenValue = 0.0
	ti.OwnerDID = ""
	ti.BlockID = ""
	
	p.pool.Put(ti)
}

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

// TokenSlicePool manages pools of token slices to reduce slice allocations
type TokenSlicePool struct {
	smallPool  sync.Pool // For slices up to 100 tokens
	mediumPool sync.Pool // For slices up to 1000 tokens
	largePool  sync.Pool // For slices up to 10000 tokens
}

// NewTokenSlicePool creates a new token slice pool
func NewTokenSlicePool() *TokenSlicePool {
	return &TokenSlicePool{
		smallPool: sync.Pool{
			New: func() interface{} {
				return make([]*contract.TokenInfo, 0, 100)
			},
		},
		mediumPool: sync.Pool{
			New: func() interface{} {
				return make([]*contract.TokenInfo, 0, 1000)
			},
		},
		largePool: sync.Pool{
			New: func() interface{} {
				return make([]*contract.TokenInfo, 0, 10000)
			},
		},
	}
}

// Get retrieves a token slice of appropriate size
func (p *TokenSlicePool) Get(size int) []*contract.TokenInfo {
	switch {
	case size <= 100:
		return p.smallPool.Get().([]*contract.TokenInfo)[:0]
	case size <= 1000:
		return p.mediumPool.Get().([]*contract.TokenInfo)[:0]
	case size <= 10000:
		return p.largePool.Get().([]*contract.TokenInfo)[:0]
	default:
		// For very large sizes, just allocate
		return make([]*contract.TokenInfo, 0, size)
	}
}

// Put returns a token slice to the appropriate pool
func (p *TokenSlicePool) Put(slice []*contract.TokenInfo) {
	// Clear the slice
	for i := range slice {
		slice[i] = nil
	}
	slice = slice[:0]
	
	cap := cap(slice)
	switch {
	case cap <= 100:
		p.smallPool.Put(slice)
	case cap <= 1000:
		p.mediumPool.Put(slice)
	case cap <= 10000:
		p.largePool.Put(slice)
	default:
		// Don't pool very large slices
	}
}