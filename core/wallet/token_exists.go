package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TokenExists checks if a token with the given ID exists in the tokens table.
// This function can be called within an existing transaction or with the main database pool.
// It returns true if the token exists, false otherwise.
//
// Parameters:
//   - ctx: Context for the database operation
//   - tx: Optional database transaction. If nil, uses the main database pool.
//   - tokenID: The token ID to check
//
// Returns:
//   - bool: true if token exists, false otherwise
//   - error: database error if any
func (w *Wallet) TokenExists(ctx context.Context, tx pgx.Tx, tokenID string) (bool, error) {
	var exists bool
	var err error

	if tx != nil {
		// Use provided transaction
		err = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id = $1)`,
			tokenID,
		).Scan(&exists)
	} else {
		// Use main database pool
		err = w.db.Pool().QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id = $1)`,
			tokenID,
		).Scan(&exists)
	}

	if err != nil {
		return false, fmt.Errorf("TokenExists: failed to check token existence: %w", err)
	}

	return exists, nil
}
