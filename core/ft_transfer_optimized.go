package core

// ft_transfer_optimized.go — dead-code stub (Phase 09 replacement target)
// Optimized FT locking path; all FT transfer logic replaced by InitiateTransaction.
// These stubs exist only to satisfy compiler.

import (
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// OptimizedFTTransferLocking stubs the optimized FT token-locking path.
func (c *Core) OptimizedFTTransferLocking(ftsForTxn []wallet.FTToken, did string, numTokens int) ([]ContractTokenInfo, error) {
	return nil, nil
}

// rollbackFTLocking stubs rollback of locked FT tokens.
func (c *Core) rollbackFTLocking(ftsForTxn []wallet.FTToken) {
}

// shouldUseOptimizedFTLocking stubs the heuristic for choosing the optimized path.
func (c *Core) shouldUseOptimizedFTLocking(ftCount int) bool {
	return false
}
