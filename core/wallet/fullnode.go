package wallet

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) GetFullNodeRBTToken(tokenID string) (models.FullNodeRBT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, latest_position, latest_role, created_at, updated_at
		 FROM fullnode_rbt WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeRBT
	if err := row.Scan(
		&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeRBT{}, fmt.Errorf("GetFullNodeRBTToken: token %v not found", tokenID)
		}
		return models.FullNodeRBT{}, fmt.Errorf("GetFullNodeRBTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeFTToken(tokenID string) (models.FullNodeFT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, latest_position, latest_role, created_at, updated_at
		 FROM fullnode_ft WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeFT
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeFT{}, fmt.Errorf("GetFullNodeFTToken: token %v not found", tokenID)
		}
		return models.FullNodeFT{}, fmt.Errorf("GetFullNodeFTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeNFTToken(tokenID string) (models.FullNodeNFT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, did, transaction_id,
		 token_state_hash,latest_position, latest_role, created_at, updated_at
		 FROM fullnode_nft WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeNFT
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeNFT{}, fmt.Errorf("GetFullNodeNFTToken: token %v not found", tokenID)
		}
		return models.FullNodeNFT{}, fmt.Errorf("GetFullNodeNFTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeSmartContractToken(tokenID string) (models.FullNodeSmartContract, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, transaction_id,
		 token_state_hash,  latest_position, latest_role, created_at, updated_at
		 FROM fullnode_smart_contract WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeSmartContract
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeSmartContract{}, fmt.Errorf("GetFullNodeSmartContractToken: token %v not found", tokenID)
		}
		return models.FullNodeSmartContract{}, fmt.Errorf("GetFullNodeSmartContractToken scan: %w", err)
	}

	return t, nil
}

// StoreInvalidTransaction stores failed validation payloads in
// fullnode_invalid_transactions. The caller is responsible for ensuring this
// is invoked at most once per transaction (e.g. only after all retries are
// exhausted) — duplicate calls will produce duplicate rows.
func (w *Wallet) StoreInvalidTransaction(transaction *models.Transactions, reason string) error {
	if transaction == nil {
		return fmt.Errorf("StoreInvalidTransaction: transaction is nil")
	}
	if reason == "" {
		return fmt.Errorf("StoreInvalidTransaction: reason is required")
	}

	payload, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Errorf("StoreInvalidTransaction: marshal transaction: %w", err)
	}

	_, err = w.db.Pool().Exec(w.Ctx, `
		INSERT INTO fullnode_invalid_transactions (transaction, reason, created_at, updated_at)
		VALUES ($1::json, $2, NOW(), NOW())
	`, payload, reason)
	if err != nil {
		return fmt.Errorf("StoreInvalidTransaction: insert invalid transaction: %w", err)
	}

	return nil
}

func (w *Wallet) GetLatestFullNodeTransactionAndRoleByTokenID(tokenID string) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT transaction_id, role FROM fullnode_tokenchain WHERE token_id = $1 ORDER BY position DESC LIMIT 1`, tokenID,
	)

	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, nil
		}
		return nil, -1, fmt.Errorf("GetLatestFullNodeTransactionByTokenID scan: %w", err)
	}

	tx, err := w.GetTransactionByID(txID, true)
	if err != nil {
		return nil, -1, fmt.Errorf("GetLatestFullNodeTransactionByTokenID GetTransactionByID: %w", err)
	}

	return tx, tokenRoleInTx, nil
}

// syncChainRow is the row shape used by GetFullNodeSyncedChainPage.
type syncChainRow struct {
	TransactionID         string          `db:"transaction_id"`
	Role                  int16           `db:"role"`
	PreviousTransactionID *string         `db:"previous_transaction_id"`
	Info                  json.RawMessage `db:"info"`
}

// GetFullNodeSyncedChainPage returns entries [offset, offset+limit) of the
// token's chain in chronological order, plus has_more indicating whether more
// entries exist past offset+limit. Uses the (token_id, position) primary key
// for an indexed range scan.
func (w *Wallet) GetFullNodeSyncedChainPage(tokenID string, offset, limit int) ([]types.SyncedTxn, bool, error) {
	if limit <= 0 {
		return []types.SyncedTxn{}, false, nil
	}

	// Fetch limit+1 to detect has_more without a second query.
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT tc.transaction_id, tc.role, tc.previous_transaction_id, t.info
		FROM fullnode_tokenchain tc
		JOIN fullnode_transactions t ON t.id = tc.transaction_id
		WHERE tc.token_id = $1
		ORDER BY tc.position ASC
		LIMIT $2 OFFSET $3
	`, tokenID, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("GetFullNodeSyncedChainPage: query: %w", err)
	}

	chainRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[syncChainRow])
	if err != nil {
		return nil, false, fmt.Errorf("GetFullNodeSyncedChainPage: collect rows: %w", err)
	}

	hasMore := len(chainRows) > limit
	if hasMore {
		chainRows = chainRows[:limit]
	}

	result := make([]types.SyncedTxn, 0, len(chainRows))
	for i := range chainRows {
		row := &chainRows[i]
		var prev string
		if row.PreviousTransactionID != nil {
			prev = *row.PreviousTransactionID
		}
		// Pass the stored Info bytes through without parsing. The explorer
		// unmarshals on its side. A corrupt row no longer fails the whole
		// page — the explorer can detect and skip it.
		result = append(result, types.SyncedTxn{
			ID:                    row.TransactionID,
			Role:                  row.Role,
			PreviousTransactionID: prev,
			Info:                  row.Info,
		})
	}

	return result, hasMore, nil
}
