package core

import (
	"context"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// PersistPostConsensus is the core-layer entry point for atomic PostgreSQL
// persistence after consensus succeeds for the local node.
//
// Use this from initiator, quorum, or receiver success paths with:
//   - req.DID set to the local node DID being persisted
//   - req.ExecutionRole set to "initiator", "quorum", or "receiver"
//   - req.TransactionInfo and req.Signature populated from the finalized transaction
//
// The coordinator writes, in one DB transaction:
//   1. transactions
//   2. transaction_units
//   3. tokenchain
//   4. tokens
//
// If req.TokenChainRows or req.TokenStates are missing, the wallet layer will
// derive them from TransactionInfo plus current PostgreSQL state.
func (c *Core) PersistPostConsensus(ctx context.Context, req *wallet.PostConsensusPersistenceRequest) error {
	return c.w.PersistPostConsensus(ctx, req)
}
