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

// syncChainRow is the raw join shape used by GetFullNodeSyncedChain.
type syncChainRow struct {
	TransactionID         string          `db:"transaction_id"`
	Role                  int16           `db:"role"`
	PreviousTransactionID *string         `db:"previous_transaction_id"`
	Info                  json.RawMessage `db:"info"`
}

// GetFullNodeSyncedChain returns the per-token transaction chain in
// chronological order for the sync-from-fullnode API. Each entry carries the
// canonical transaction ID, the role this transaction plays for this token in
// fullnode_tokenchain, and the previous_transaction_id stored there — none of
// which are derivable from TransactionInfo alone (unpledge entries reuse some
// other transaction's Info, so the unpledged token's previous-tx pointer lives
// only in fullnode_tokenchain).
func (w *Wallet) GetFullNodeSyncedChain(tokenID string) ([]types.SyncedTxn, error) {
	var indices []int32
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT index FROM fullnode_tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&indices)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GetFullNodeSyncedChain: no fullnode_tokenchain_index found for token_id %s", tokenID)
		}
		return nil, fmt.Errorf("GetFullNodeSyncedChain: failed to get indices: %w", err)
	}

	if len(indices) == 0 {
		return []types.SyncedTxn{}, nil
	}

	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT tc.transaction_id, tc.role, tc.previous_transaction_id, t.info
		FROM unnest($1::int[]) WITH ORDINALITY AS idx(id, ord)
		JOIN fullnode_tokenchain tc ON tc.id = idx.id
		JOIN fullnode_transactions t ON t.id = tc.transaction_id
		ORDER BY idx.ord
	`, indices)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeSyncedChain: failed to query chain rows: %w", err)
	}

	chainRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[syncChainRow])
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeSyncedChain: failed to collect rows: %w", err)
	}

	result := make([]types.SyncedTxn, 0, len(chainRows))
	for i := range chainRows {
		row := &chainRows[i]
		var info models.TransactionInfo
		if err := json.Unmarshal(row.Info, &info); err != nil {
			return nil, fmt.Errorf("GetFullNodeSyncedChain: failed to parse Info for txID %q (token %q): %w", row.TransactionID, tokenID, err)
		}
		var prev string
		if row.PreviousTransactionID != nil {
			prev = *row.PreviousTransactionID
		}
		result = append(result, types.SyncedTxn{
			ID:                    row.TransactionID,
			Role:                  row.Role,
			PreviousTransactionID: prev,
			Info:                  &info,
		})
	}

	return result, nil
}
