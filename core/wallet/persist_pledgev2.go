package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// PersistPledgeV2 atomically persists a pledge transaction into the database.
//
// It is STANDALONE — it does NOT call PersistPostConsensus or
// validateTransferChainContinuity. Idempotency is provided by ON CONFLICT DO
// NOTHING guards on the transactions and tokenchain tables.
//
// Steps performed inside a single DB transaction:
//  1. Insert into transactions (ON CONFLICT DO NOTHING)
//  2. Insert into transaction_units (ON CONFLICT DO NOTHING)
//  3. Batch insert into tokenchain (ON CONFLICT DO NOTHING)
//  4. Rebuild tokenchain_index (full-rebuild, idempotent)
//  5. Upsert token states in tokens table
func (w *Wallet) PersistPledgeV2(
	ctx context.Context,
	pledgeTxInfo *models.TransactionInfo,
	signature *models.Signature,
	pledgeTxID string,
	quorumDID string,
	tokenChainRows []models.TokenChain,
	tokenStates []models.Token,
	affectedTokens []string,
) error {
	return w.persistPledgeUnpledgeV2(ctx, pledgeTxInfo, signature, pledgeTxID, quorumDID,
		ExecutionRoleQuorum, tokenChainRows, tokenStates, affectedTokens)
}

// PersistUnpledgeV2 atomically persists an unpledge transaction into the database.
//
// It is STANDALONE — it does NOT call PersistPostConsensus or
// validateTransferChainContinuity. Idempotency is provided by ON CONFLICT DO
// NOTHING guards on the transactions and tokenchain tables.
func (w *Wallet) PersistUnpledgeV2(
	ctx context.Context,
	unpledgeTxInfo *models.TransactionInfo,
	signature *models.Signature,
	unpledgeTxID string,
	quorumDID string,
	tokenChainRows []models.TokenChain,
	tokenStates []models.Token,
	affectedTokens []string,
) error {
	return w.persistPledgeUnpledgeV2(ctx, unpledgeTxInfo, signature, unpledgeTxID, quorumDID,
		ExecutionRoleQuorum, tokenChainRows, tokenStates, affectedTokens)
}

// persistPledgeUnpledgeV2 is the shared implementation for PersistPledgeV2 and
// PersistUnpledgeV2. It opens its own DB transaction and performs all SQL in one
// atomic unit.
func (w *Wallet) persistPledgeUnpledgeV2(
	ctx context.Context,
	txInfo *models.TransactionInfo,
	signature *models.Signature,
	txID string,
	did string,
	executionRole string,
	tokenChainRows []models.TokenChain,
	tokenStates []models.Token,
	affectedTokens []string,
) error {
	if txInfo == nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: txInfo is required")
	}
	if signature == nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: signature is required")
	}
	if txID == "" {
		return fmt.Errorf("persistPledgeUnpledgeV2: txID is required")
	}
	if did == "" {
		return fmt.Errorf("persistPledgeUnpledgeV2: did is required")
	}
	if len(tokenChainRows) == 0 {
		return fmt.Errorf("persistPledgeUnpledgeV2: tokenChainRows is required")
	}
	if len(tokenStates) == 0 {
		return fmt.Errorf("persistPledgeUnpledgeV2: tokenStates is required")
	}
	if len(affectedTokens) == 0 {
		return fmt.Errorf("persistPledgeUnpledgeV2: affectedTokens is required")
	}

	// Serialize txInfo and signature for storage.
	txInfoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: serialize txInfo: %w", err)
	}
	sigBytes, err := json.Marshal(signature)
	if err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: marshal signature: %w", err)
	}

	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Insert into transactions (ON CONFLICT DO NOTHING for idempotency).
	if _, err := tx.Exec(ctx, `
		INSERT INTO transactions (id, info, signature, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, txID, txInfoBytes, sigBytes); err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: insert transaction %q: %w", txID, err)
	}

	// Step 2: Insert into transaction_units.
	if _, err := tx.Exec(ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, txID, did, executionRole, transactionUnitStatusCommitted); err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: insert transaction unit for tx %q, did %q: %w", txID, did, err)
	}

	// Step 3: Batch insert tokenchain rows.
	const batchSize = 250
	for start := 0; start < len(tokenChainRows); start += batchSize {
		end := start + batchSize
		if end > len(tokenChainRows) {
			end = len(tokenChainRows)
		}
		chunk := tokenChainRows[start:end]

		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*5)
		for i, row := range chunk {
			offset := i*5 + 1
			placeholders = append(placeholders,
				fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, NOW(), NOW())",
					offset, offset+1, offset+2, offset+3, offset+4,
				),
			)
			args = append(args,
				row.TokenID,
				row.TransactionID,
				row.PreviousTransactionID,
				row.Role,
				row.Position,
			)
		}
		query := `
			INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			VALUES ` + strings.Join(placeholders, ",") + `
			ON CONFLICT (token_id, position) DO NOTHING
		`
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("persistPledgeUnpledgeV2: insert tokenchain rows for tx %q: %w", txID, err)
		}
	}

	// Step 4: Rebuild tokenchain_index (full rebuild — idempotent).
	indexRows, err := tx.Query(ctx, `
		SELECT token_id, array_agg(id ORDER BY position)
		FROM tokenchain
		WHERE token_id = ANY($1::text[])
		GROUP BY token_id
	`, affectedTokens)
	if err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: read tokenchain index for tx %q: %w", txID, err)
	}
	defer indexRows.Close()

	type tokenIndexRow struct {
		tokenID string
		index   []int32
	}
	idxEntries := make([]tokenIndexRow, 0, len(affectedTokens))
	for indexRows.Next() {
		var entry tokenIndexRow
		if err := indexRows.Scan(&entry.tokenID, &entry.index); err != nil {
			return fmt.Errorf("persistPledgeUnpledgeV2: scan tokenchain index for tx %q: %w", txID, err)
		}
		idxEntries = append(idxEntries, entry)
	}
	if err := indexRows.Err(); err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: stream tokenchain index for tx %q: %w", txID, err)
	}
	indexRows.Close()

	if len(idxEntries) > 0 {
		placeholders := make([]string, 0, len(idxEntries))
		args := make([]any, 0, len(idxEntries)*2)
		for i, entry := range idxEntries {
			offset := i*2 + 1
			placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, NOW(), NOW())", offset, offset+1))
			args = append(args, entry.tokenID, entry.index)
		}
		indexQuery := `
			INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
			VALUES ` + strings.Join(placeholders, ",") + `
			ON CONFLICT (token_id) DO UPDATE SET
				index = EXCLUDED.index,
				updated_at = NOW()
		`
		if _, err := tx.Exec(ctx, indexQuery, args...); err != nil {
			return fmt.Errorf("persistPledgeUnpledgeV2: upsert tokenchain_index for tx %q: %w", txID, err)
		}
	}

	// Step 5: Upsert token states.
	for start := 0; start < len(tokenStates); start += batchSize {
		end := start + batchSize
		if end > len(tokenStates) {
			end = len(tokenStates)
		}
		chunk := tokenStates[start:end]

		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*10)
		for i, state := range chunk {
			offset := i*10 + 1
			placeholders = append(placeholders,
				fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW(), NOW())",
					offset, offset+1, offset+2, offset+3, offset+4,
					offset+5, offset+6, offset+7, offset+8, offset+9,
				),
			)
			args = append(args,
				state.TokenID,
				state.ParentTokenID,
				state.TokenValue,
				state.TokenStatus,
				state.DID,
				state.TransactionID,
				state.TokenStateHash,
				state.TokenType,
				state.LatestPosition,
				state.LatestRole,
			)
		}
		stateQuery := `
			INSERT INTO tokens (
				token_id, parent_token_id, token_value, token_status, did, transaction_id,
				token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
			)
			VALUES ` + strings.Join(placeholders, ",") + `
			ON CONFLICT (token_id) DO UPDATE SET
				token_status = EXCLUDED.token_status,
				did = EXCLUDED.did,
				transaction_id = EXCLUDED.transaction_id,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`
		if _, err := tx.Exec(ctx, stateQuery, args...); err != nil {
			return fmt.Errorf("persistPledgeUnpledgeV2: upsert token states for tx %q: %w", txID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("persistPledgeUnpledgeV2: commit tx %q: %w", txID, err)
	}

	return nil
}
