package wallet

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// ReadLatestTokenChainRows is an exported wrapper for the unexported
// readLatestTokenChainRows method. It returns the most recent tokenchain row
// for each token ID in tokenIDs, keyed by token ID.
func (w *Wallet) ReadLatestTokenChainRows(ctx context.Context, tokenIDs []string) (map[string]*models.TokenChain, error) {
	return w.readLatestTokenChainRows(ctx, tokenIDs)
}

// ReadTokensByIDs is an exported wrapper for the unexported readTokensByIDs
// method. It returns current token records keyed by token ID.
func (w *Wallet) ReadTokensByIDs(ctx context.Context, tokenIDs []string) (map[string]models.Token, error) {
	return w.readTokensByIDs(ctx, tokenIDs)
}

// Pool returns the underlying pgxpool.Pool for direct query access.
// Used by Core-level methods (PledgeV2, UnpledgeV2) that need to run
// raw queries outside of a wallet-managed transaction.
func (w *Wallet) Pool() *pgxpool.Pool {
	return w.db.Pool()
}
