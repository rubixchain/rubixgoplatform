package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) GetTokenChainByTokenID(tokenID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at
		 FROM tokenchain WHERE token_id = $1 ORDER BY position ASC`, tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenID: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

func (w *Wallet) GetLatestTransactionAndRoleByTokenID(tokenID string) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
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

	tx, err := w.GetTransactionByID(txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetLatestTransactionByTokenID GetTransactionByID: %w", err)
	}

	return tx, tokenRoleInTx, nil
}

func (w *Wallet) AddTokenChainEntry(entry *models.TokenChain) error {
	_, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO tokenchain (token_id, transaction_id,previous_transaction_id, role, position, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position)
	if err != nil {
		return fmt.Errorf("AddTokenChainEntry: %w", err)
	}
	return nil
}

func (w *Wallet) GetTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx, `
		SELECT transaction_id,role FROM tokenchain WHERE token_id = $1 AND position = $2
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

	tx, err := w.GetTransactionByID(txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetTransactionAtHeight transaction details not found for transaction_id: %v, err %w", txID, err)
	}

	return tx, tokenRoleInTx, nil
}

func (w *Wallet) GetTokenChainByTransactionID(transactionID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, transaction_id, role, position, created_at, updated_at
		 FROM tokenchain WHERE transaction_id = $1 ORDER BY position`, transactionID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTransactionID: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// PersistGenesisTokenRecord atomically inserts a genesis transaction, token, and tokenchain
// entry in a single PostgreSQL transaction. All inserts are idempotent (ON CONFLICT DO NOTHING).
// Rollback is automatic on any failure via deferred Rollback.
func (w *Wallet) PersistGenesisTokenRecord(
	txRecord *models.Transactions,
	token *models.Token,
	entry *models.TokenChain,
) error {
	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
	}
	defer tx.Rollback(w.Ctx) //nolint:errcheck

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		txRecord.ID, txRecord.Info, txRecord.Signature,
	); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert transaction: %w", err)
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		 ON CONFLICT (token_id) DO UPDATE SET
		   transaction_id = EXCLUDED.transaction_id,
		   token_state_hash = EXCLUDED.token_state_hash,
		   latest_position = EXCLUDED.latest_position,
		   latest_role = EXCLUDED.latest_role,
		   updated_at = NOW()`,
		token.TokenID, token.ParentTokenID, token.TokenValue, token.TokenStatus,
		token.DID, token.TransactionID, token.TokenStateHash, token.TokenType,
		token.LatestPosition, token.LatestRole,
	); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert token: %w", err)
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (token_id, position) DO NOTHING`,
		entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position,
	); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert tokenchain: %w", err)
	}

	return tx.Commit(w.Ctx)
}
