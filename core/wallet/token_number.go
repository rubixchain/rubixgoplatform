package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetNextTokenNumber atomically increments the global token counter and returns
// the new value. Must be called within an existing pgx.Tx from
// PersistGenesisBatch or PersistGenesisTokenRecord — never from callers outside
// those two functions.
//
// On first call (no row for attribute='token_number') the counter starts at 1.
func (w *Wallet) GetNextTokenNumber(ctx context.Context, tx pgx.Tx) (int, error) {
	var value int

	err := tx.QueryRow(ctx,
		`UPDATE local_test_token_info
		 SET value = value + 1, updated_at = NOW()
		 WHERE attribute = 'token_number'
		 RETURNING value`,
	).Scan(&value)

	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx,
			`INSERT INTO local_test_token_info (attribute, value)
			 VALUES ('token_number', 1)
			 RETURNING value`,
		).Scan(&value)
	}

	if err != nil {
		return 0, fmt.Errorf("GetNextTokenNumber: %w", err)
	}
	return value, nil
}
