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

// syncChainRow is one row from the fullnode_tokenchain + fullnode_transactions
// join used by the sync-txn-info-chain page query.
type syncChainRow struct {
	TokenID               string          `db:"token_id"`
	TransactionID         string          `db:"transaction_id"`
	Role                  int16           `db:"role"`
	Position              int64           `db:"position"`
	PreviousTransactionID *string         `db:"previous_transaction_id"`
	Info                  json.RawMessage `db:"info"`
}

// DetectDivergentSyncTokens returns the tokens whose KnownPositions claim
// doesn't match the fullnode's chain at the claimed position. A token is
// divergent if either no row exists at (token_id, position) or the row's
// tx_id differs from the claim.
//
// The handler calls this first, then drops divergent tokens from the
// thresholds map passed to Count/GetPage so those tokens default back to
// "full chain from position 0".
func (w *Wallet) DetectDivergentSyncTokens(known map[string]types.TokenChainTip) ([]string, error) {
	if len(known) == 0 {
		return nil, nil
	}
	tokenIDs := make([]string, 0, len(known))
	positions := make([]int64, 0, len(known))
	claimedTxIDs := make([]string, 0, len(known))
	for tokenID, tip := range known {
		if tokenID == "" {
			continue
		}
		tokenIDs = append(tokenIDs, tokenID)
		positions = append(positions, tip.Position)
		claimedTxIDs = append(claimedTxIDs, tip.TransactionID)
	}
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT input.token_id
		FROM unnest($1::text[], $2::bigint[], $3::text[]) AS input(token_id, position, claimed_tx_id)
		LEFT JOIN fullnode_tokenchain tc
		  ON tc.token_id = input.token_id AND tc.position = input.position
		WHERE tc.transaction_id IS NULL OR tc.transaction_id <> input.claimed_tx_id
	`, tokenIDs, positions, claimedTxIDs)
	if err != nil {
		return nil, fmt.Errorf("DetectDivergentSyncTokens: %w", err)
	}
	defer rows.Close()

	divergent := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("DetectDivergentSyncTokens scan: %w", err)
		}
		divergent = append(divergent, t)
	}
	return divergent, rows.Err()
}

// CountFullNodeSyncedChainEntries returns how many chain entries match the
// filter, so the handler can compute total_pages. For each token_id, only
// entries with `position > thresholds[token_id]` are counted. Tokens not in
// the thresholds map default to -1 (full chain).
func (w *Wallet) CountFullNodeSyncedChainEntries(tokenIDs []string, thresholds map[string]int64) (int, error) {
	if len(tokenIDs) == 0 {
		return 0, nil
	}
	tokenIDsArg, thresholdsArg := buildPerTokenThresholds(tokenIDs, thresholds)
	if len(tokenIDsArg) == 0 {
		return 0, nil
	}

	var count int
	err := w.db.Pool().QueryRow(w.Ctx, `
		SELECT COUNT(*)
		FROM fullnode_tokenchain tc
		JOIN unnest($1::text[], $2::bigint[]) AS f(token_id, threshold)
		  ON f.token_id = tc.token_id
		WHERE tc.position > f.threshold
	`, tokenIDsArg, thresholdsArg).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountFullNodeSyncedChainEntries: %w", err)
	}
	return count, nil
}

// GetFullNodeSyncedChainPageByOffset returns one page of chain entries,
// ordered by (token_id, position). Per-token thresholds drop entries the
// caller already has. Tokens not in the thresholds map default to -1 so
// their full chain is included.
//
// Returns parallel slices (tokenIDs[i], entries[i]) so the handler can group
// rows by token in the response without a second query.
func (w *Wallet) GetFullNodeSyncedChainPageByOffset(
	tokenIDs []string,
	thresholds map[string]int64,
	offset, limit int,
) ([]string, []types.SyncedTxn, error) {
	if limit <= 0 || len(tokenIDs) == 0 {
		return nil, nil, nil
	}
	tokenIDsArg, thresholdsArg := buildPerTokenThresholds(tokenIDs, thresholds)
	if len(tokenIDsArg) == 0 {
		return nil, nil, nil
	}

	// NOTE (replayed-split check, Option A — DISABLED): this SELECT deliberately
	// omits t.signature, so SyncedTxn carries no signature. The only current
	// consumer (ValidateSplitParentsAgainstFullnode) does not persist the chain,
	// so it does not need it. To enable Option A (applying the synced chain into a
	// signature-NOT-NULL transactions table), add `, t.signature` to the SELECT,
	// add a Signature field to syncChainRow, and set SyncedTxn.Signature in the
	// SyncedTxn build below — mirroring the recovery query in
	// core/wallet/recovery.go which selects t.info AND t.signature.
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT tc.token_id, tc.transaction_id, tc.role, tc.position,
		       tc.previous_transaction_id, t.info
		FROM fullnode_tokenchain tc
		JOIN fullnode_transactions t ON t.id = tc.transaction_id
		JOIN unnest($1::text[], $2::bigint[]) AS f(token_id, threshold)
		  ON f.token_id = tc.token_id
		WHERE tc.position > f.threshold
		ORDER BY tc.token_id ASC, tc.position ASC
		LIMIT $3 OFFSET $4
	`, tokenIDsArg, thresholdsArg, limit, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("GetFullNodeSyncedChainPageByOffset: query: %w", err)
	}

	chainRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[syncChainRow])
	if err != nil {
		return nil, nil, fmt.Errorf("GetFullNodeSyncedChainPageByOffset: collect rows: %w", err)
	}

	keys := make([]string, 0, len(chainRows))
	out := make([]types.SyncedTxn, 0, len(chainRows))
	for i := range chainRows {
		row := &chainRows[i]
		var prev string
		if row.PreviousTransactionID != nil {
			prev = *row.PreviousTransactionID
		}
		keys = append(keys, row.TokenID)
		out = append(out, types.SyncedTxn{
			ID:                    row.TransactionID,
			Role:                  row.Role,
			Position:              row.Position,
			PreviousTransactionID: prev,
			Info:                  row.Info,
		})
	}
	return keys, out, nil
}

// buildPerTokenThresholds flattens (tokenIDs, thresholds map) into the two
// parallel arrays the SQL unnest() pattern expects. Empty token IDs are
// dropped. Tokens missing from the map get -1, which means "no threshold,
// include the full chain".
func buildPerTokenThresholds(tokenIDs []string, thresholds map[string]int64) ([]string, []int64) {
	tokensOut := make([]string, 0, len(tokenIDs))
	thresholdsOut := make([]int64, 0, len(tokenIDs))
	for _, t := range tokenIDs {
		if t == "" {
			continue
		}
		threshold := int64(-1)
		if v, ok := thresholds[t]; ok {
			threshold = v
		}
		tokensOut = append(tokensOut, t)
		thresholdsOut = append(thresholdsOut, threshold)
	}
	return tokensOut, thresholdsOut
}
