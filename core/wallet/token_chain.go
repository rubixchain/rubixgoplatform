package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) GetTokenChainByTokenID(ctx context.Context, tokenID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(ctx,
		`SELECT token_id, transaction_id, role, height, created_at, updated_at
		 FROM tokenchain WHERE token_id = $1 ORDER BY position`, tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenID: %w", err)
	}
	defer rows.Close()

	var entries []models.TokenChain
	for rows.Next() {
		var tc models.TokenChain
		if err := rows.Scan(&tc.TokenID, &tc.TransactionID, &tc.Role, &tc.Height, &tc.CreatedAt, &tc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("GetTokenChainByTokenID scan: %w", err)
		}
		entries = append(entries, tc)
	}

	return entries, rows.Err()
}

func (w *Wallet) GetLatestTransactionAndRoleByTokenID(ctx context.Context, tokenID string) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(ctx,
		`SELECT transaction_id, role FROM tokenchain WHERE token_id = $1 ORDER BY position DESC LIMIT 1`, tokenID,
	)

	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, nil
		}
		return nil, -1, fmt.Errorf("GetLatestTransactionByTokenID scan: %w", err)
	}

	tx, err := w.GetTransactionByID(ctx, txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetLatestTransactionByTokenID GetTransactionByID: %w", err)
	}
	
	return tx, tokenRoleInTx, nil
}

func (w *Wallet) AddTokenChainEntry(ctx context.Context, entry *models.TokenChain) error {
	_, err := w.db.Pool().Exec(ctx, `
		INSERT INTO tokenchain (token_id, transaction_id, role, height, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, entry.TokenID, entry.TransactionID, entry.Role, entry.Height)
	if err != nil {
		return fmt.Errorf("AddTokenChainEntry: %w", err)
	}
	return nil
}

func (w *Wallet) GetTransactionAndRoleAtHeight(ctx context.Context, tokenID string, height int64) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(ctx,`
		SELECT transaction_id FROM tokenchain WHERE token_id = $1 AND height = $2 
		ORDER BY created_at DESC LIMIT 1`, tokenID, height,
	)
	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, fmt.Errorf("transaction not found at height %d for token %s", height, tokenID)
		}
		return nil, -1, fmt.Errorf("GetTransactionAtHeight scan: %w", err)
	}

	tx, err := w.GetTransactionByID(ctx, txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetTransactionAtHeight transaction details not found for transaction_id: %v, err %w", txID, err)
	}

	return tx, tokenRoleInTx, nil
}