package core

import (
	"sync"
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
				return &ContractTokenInfo{}
			},
		},
	}
}

// Get retrieves a TokenInfo from the pool
func (p *TokenInfoPool) Get() *ContractTokenInfo {
	return p.pool.Get().(*ContractTokenInfo)
}

// Put returns a TokenInfo to the pool after resetting it
func (p *TokenInfoPool) Put(ti *ContractTokenInfo) {
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
				return make([]*ContractTokenInfo, 0, 100)
			},
		},
		mediumPool: sync.Pool{
			New: func() interface{} {
				return make([]*ContractTokenInfo, 0, 1000)
			},
		},
		largePool: sync.Pool{
			New: func() interface{} {
				return make([]*ContractTokenInfo, 0, 10000)
			},
		},
	}
}

// Get retrieves a token slice of appropriate size
func (p *TokenSlicePool) Get(size int) []*ContractTokenInfo {
	switch {
	case size <= 100:
		return p.smallPool.Get().([]*ContractTokenInfo)[:0]
	case size <= 1000:
		return p.mediumPool.Get().([]*ContractTokenInfo)[:0]
	case size <= 10000:
		return p.largePool.Get().([]*ContractTokenInfo)[:0]
	default:
		// For very large sizes, just allocate
		return make([]*ContractTokenInfo, 0, size)
	}
}

// Put returns a token slice to the appropriate pool
func (p *TokenSlicePool) Put(slice []*ContractTokenInfo) {
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