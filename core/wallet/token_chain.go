package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	tokenmap "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// GenesisMintRecord groups the three tables' data for a single genesis token
// into one unit that PersistGenesisBatch inserts atomically.
// Token.TokenStateHash must be set to the IPFS CID from w.Add before calling
// PersistGenesisBatch (set explicitly after w.Pin succeeds in createChildTokensAtLevel).
type GenesisMintRecord struct {
	TxRecord   *models.Transactions
	Token      *models.Token
	TokenChain *models.TokenChain
}

// PersistGenesisBatch atomically inserts N genesis tokens across five tables
// (transactions, tokens, tokenchain, tokenchain_index, transaction_units) in
// a single pgx.Tx. Optionally updates the denom array for did in the same Tx.
//
// Invariant guards (applied before any DB work):
//   - TokenChain.Position == 0
//   - TokenChain.Role == mint role
//   - TokenChain.PreviousTransactionID == nil
//   - TxRecord.Signature non-empty
func (w *Wallet) PersistGenesisBatch(
	ctx context.Context,
	records []GenesisMintRecord,
	did string,
	denomMap map[types.DenomValue]types.DenomCount,
) error {
	if len(records) == 0 {
		return nil
	}

	mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))

	// Fail fast: validate all invariants before opening a DB transaction.
	for i, r := range records {
		if r.TxRecord.ID == "" {
			return fmt.Errorf("genesis: record[%d]: empty transaction_id", i)
		}
		if r.TokenChain.Position != 0 {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: TokenChain.Position must be 0 for genesis, got %d", i, r.TokenChain.Position)
		}
		if r.TokenChain.Role != mintRoleID {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: TokenChain.Role must be mint (%d) for genesis, got %d", i, mintRoleID, r.TokenChain.Role)
		}
		if r.TokenChain.PreviousTransactionID != nil {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: TokenChain.PreviousTransactionID must be nil for genesis, got %q", i, *r.TokenChain.PreviousTransactionID)
		}
		if len(r.TxRecord.Signature) == 0 {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: TxRecord.Signature must not be empty for genesis", i)
		}
		if r.Token.TokenStatus == constants.TokenStatus_Free && len(r.TxRecord.Signature) == 0 {
			return fmt.Errorf("genesis: FREE token must have initiator signature")
		}
		// TODO(phase09-sig): verify signature against r.TxRecord inputs when pubkey available
	}

	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("PersistGenesisBatch: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert into transactions, tokens, tokenchain for each record.
	for i, r := range records {
		// Generate canonical TokenID from global DB counter if the caller left it empty.
		if r.Token.TokenID == "" {
			globalIndex, err := w.GetNextTokenNumber(ctx, tx)
			if err != nil {
				return fmt.Errorf("PersistGenesisBatch: record[%d]: GetNextTokenNumber: %w", i, err)
			}
			tokenLevel, numInLevel, err := tokenmap.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
			if err != nil {
				return fmt.Errorf("PersistGenesisBatch: record[%d]: GetTokenLevelAndNumberForGlobalIndex(%d): %w", i, globalIndex, err)
			}
			assignedID := fmt.Sprintf("%d_%d", tokenLevel, numInLevel)
			r.Token.TokenID = assignedID
			r.TokenChain.TokenID = assignedID
		}

		// Post-assignment invariants (after potential auto-assign).
		if r.Token.TokenID == "" {
			return fmt.Errorf("genesis: record[%d]: empty token_id", i)
		}
		if r.Token.TransactionID != r.TxRecord.ID {
			return fmt.Errorf("genesis: record[%d]: token %q transaction_id mismatch", i, r.Token.TokenID)
		}
		if r.TokenChain.TransactionID != r.TxRecord.ID {
			return fmt.Errorf("genesis: record[%d]: entry transaction_id mismatch for token %q", i, r.TokenChain.TokenID)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO transactions (id, info, signature, created_at, updated_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (id) DO NOTHING`,
			r.TxRecord.ID, r.TxRecord.Info, r.TxRecord.Signature,
		); err != nil {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: insert transaction: %w", i, err)
		}

		cmdTagToken, err := tx.Exec(ctx,
			`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id,
			 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
			 ON CONFLICT (token_id) DO NOTHING`,
			r.Token.TokenID, r.Token.ParentTokenID, r.Token.TokenValue, r.Token.TokenStatus,
			r.Token.DID, r.Token.TransactionID, r.Token.TokenStateHash, r.Token.TokenType,
			r.Token.LatestPosition, r.Token.LatestRole,
		)
		if err != nil {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: insert token: %w", i, err)
		}
		if cmdTagToken.RowsAffected() == 0 {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: token %q already exists — duplicate genesis call rejected", i, r.Token.TokenID)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			 ON CONFLICT (token_id, position) DO NOTHING`,
			r.TokenChain.TokenID, r.TokenChain.TransactionID, r.TokenChain.PreviousTransactionID, r.TokenChain.Role, r.TokenChain.Position,
		); err != nil {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: insert tokenchain: %w", i, err)
		}
	}

	// Batch upsert tokenchain_index using the syncTokenChainIndex pattern.
	tokenIDs := make([]string, len(records))
	for i, r := range records {
		tokenIDs[i] = r.Token.TokenID
	}
	if err := w.batchUpsertTokenChainIndex(ctx, tx, tokenIDs); err != nil {
		return fmt.Errorf("PersistGenesisBatch: upsert tokenchain_index: %w", err)
	}

	// Insert transaction_units for each record.
	for i, r := range records {
		if _, err := tx.Exec(ctx, `
			INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (transaction_id, did) DO NOTHING
		`, r.TxRecord.ID, r.Token.DID, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
			return fmt.Errorf("PersistGenesisBatch: record[%d]: insert transaction_units: %w", i, err)
		}
	}

	// Optionally update denom array within the same transaction.
	if len(denomMap) > 0 {
		if err := w.updateTokenDenomArrayTx(ctx, tx, did, denomMap); err != nil {
			return fmt.Errorf("PersistGenesisBatch: update denom array: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// batchUpsertTokenChainIndex rebuilds tokenchain_index for the given tokenIDs
// inside the caller's transaction (replicates syncTokenChainIndex pattern from
// PostConsensusPersistenceCoordinator).
func (w *Wallet) batchUpsertTokenChainIndex(ctx context.Context, tx pgx.Tx, tokenIDs []string) error {
	rows, err := tx.Query(ctx, `
		SELECT token_id, array_agg(id ORDER BY position)
		FROM tokenchain
		WHERE token_id = ANY($1::text[])
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return fmt.Errorf("batchUpsertTokenChainIndex: query: %w", err)
	}
	defer rows.Close()

	type indexRow struct {
		tokenID string
		index   []int32
	}

	indexRows := make([]indexRow, 0, len(tokenIDs))
	for rows.Next() {
		var r indexRow
		if err := rows.Scan(&r.tokenID, &r.index); err != nil {
			return fmt.Errorf("batchUpsertTokenChainIndex: scan: %w", err)
		}
		indexRows = append(indexRows, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("batchUpsertTokenChainIndex: stream: %w", err)
	}
	if len(indexRows) == 0 {
		return nil
	}

	placeholders := make([]string, 0, len(indexRows))
	args := make([]interface{}, 0, len(indexRows)*2)
	for i, r := range indexRows {
		offset := i*2 + 1
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, NOW(), NOW())", offset, offset+1))
		args = append(args, r.tokenID, r.index)
	}

	query := `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ` + strings.Join(placeholders, ",") + `
		ON CONFLICT (token_id) DO UPDATE SET
			index = EXCLUDED.index,
			updated_at = NOW()
	`
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("batchUpsertTokenChainIndex: upsert: %w", err)
	}

	return nil
}

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

// GetTokenChainByTokenIDAndPrevTxnId fetches the token chain from the input transaction id, with a limit of 100 transactions
func (w *Wallet) GetTokenChainByTokenIDAndPrevTxnId(tokenID string, txnID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT tc.*
			FROM tokenchain tc
			WHERE tc.token_id = $1
			AND tc.position >= (
				SELECT position
				FROM tokenchain
				WHERE token_id = $1
				AND previous_transaction_id = $2
				)
			ORDER BY tc.position
			LIMIT 100`, tokenID, txnID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTokenIDAndPrevTxnId: %w", err)
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

// GetRoleOfTokenIdInLatestTxn returns the token role in the latest transaction for a given token ID.
func (w *Wallet) GetRoleOfTokenIdInLatestTxn(tokenID string, isFullNode bool) (int16, error) {
	var row pgx.Row
	var err error
	if isFullNode {
		row, err = w.db.Pool().Query(w.Ctx,
			`SELECT role FROM fullnode_tokenchain WHERE token_id = $1 ORDER BY position DESC LIMIT 1`,
			tokenID,
		)
	} else {
		row, err = w.db.Pool().Query(w.Ctx,
			`SELECT role FROM %tokenchain WHERE token_id = $1 ORDER BY position DESC LIMIT 1`,
			tokenID,
		)
	}
	if err != nil {
		return -1, fmt.Errorf("GetRoleOfTokenIdInLatestTxn: %w", err)
	}
	var role int16
	if err := row.Scan(&role); err != nil {
		if err == pgx.ErrNoRows {
			return -1, fmt.Errorf("latest token role not found for token %s", tokenID)
		}
		return -1, fmt.Errorf("GetRoleOfTokenIdInLatestTxn scan: %w", err)
	}
	return role, nil
}

func (w *Wallet) AddTokenChainEntry(entry *models.TokenChain) error {
	_, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position)
	if err != nil {
		return fmt.Errorf("AddTokenChainEntry: %w", err)
	}

	// Keep tokenchain_index in sync after inserting a new tokenchain row.
	var index []int32
	if err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT array_agg(id ORDER BY position) FROM tokenchain WHERE token_id = $1`,
		entry.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("AddTokenChainEntry: query tokenchain_index: %w", err)
	}
	if _, err := w.db.Pool().Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
		  index = EXCLUDED.index,
		  updated_at = NOW()
	`, entry.TokenID, index); err != nil {
		return fmt.Errorf("AddTokenChainEntry: upsert tokenchain_index: %w", err)
	}

	return nil
}

// ApplyTokenChainBatch atomically inserts a batch of tokenchain entries for a
// single token and rebuilds its tokenchain_index. All inserts happen inside a
// single DB transaction — a partial crash rolls back every row, leaving the
// chain consistent.
//
// Safety properties enforced:
//  1. pg_advisory_xact_lock serializes concurrent callers for the same tokenID.
//  2. SELECT FOR UPDATE on the current tail prevents concurrent appends.
//  3. Cross-batch boundary check: entries[0].PreviousTransactionID must match DB tail txID.
//  4. Within-batch continuity: entries[i].PreviousTransactionID must equal entries[i-1].TransactionID.
//  5. Canonical positions are recomputed from the DB tail — caller-supplied positions are ignored.
//  6. INSERT ON CONFLICT DO NOTHING with RowsAffected check: same txID at same position is idempotent;
//     different txID at same position returns a fork error.
//  7. batchUpsertTokenChainIndex is called only when at least one new row was inserted.
//  8. Any error causes full transaction rollback with zero partial writes.
func (w *Wallet) ApplyTokenChainBatch(ctx context.Context, tokenID string, entries []*models.TokenChain) error {
	if len(entries) == 0 {
		return nil
	}

	// Step 1: Begin transaction.
	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("ApplyTokenChainBatch: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 2: Acquire advisory lock to serialize concurrent callers for this token.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, tokenID); err != nil {
		return fmt.Errorf("ApplyTokenChainBatch: advisory lock for token %s: %w", tokenID, err)
	}

	// Step 3: Row lock + read tail — get the current chain tip with FOR UPDATE.
	var basePosition int64 = -1
	var lastTxID string
	hasTail := true
	err = tx.QueryRow(ctx,
		`SELECT position, transaction_id FROM tokenchain WHERE token_id = $1 ORDER BY position DESC LIMIT 1 FOR UPDATE`,
		tokenID,
	).Scan(&basePosition, &lastTxID)
	if err != nil {
		if err == pgx.ErrNoRows {
			hasTail = false
			basePosition = -1
		} else {
			return fmt.Errorf("ApplyTokenChainBatch: fetch tail for token %s: %w", tokenID, err)
		}
	}

	// Step 4: Cross-batch boundary check.
	if hasTail {
		if entries[0].PreviousTransactionID == nil {
			return fmt.Errorf("ApplyTokenChainBatch: chain boundary mismatch for token %s: position > 0 requires a previous txID but got nil", tokenID)
		}
		if *entries[0].PreviousTransactionID != lastTxID {
			return fmt.Errorf("ApplyTokenChainBatch: chain boundary mismatch for token %s: expected previous txID %s, got %s", tokenID, lastTxID, *entries[0].PreviousTransactionID)
		}
	}

	// Step 5: Within-batch continuity check (for entries beyond the first).
	for i := 1; i < len(entries); i++ {
		prevTxID := "<nil>"
		if entries[i].PreviousTransactionID != nil {
			prevTxID = *entries[i].PreviousTransactionID
		}
		if entries[i].PreviousTransactionID == nil || *entries[i].PreviousTransactionID != entries[i-1].TransactionID {
			return fmt.Errorf("ApplyTokenChainBatch: invalid chain continuity at index %d for token %s: expected %s, got %s", i, tokenID, entries[i-1].TransactionID, prevTxID)
		}
	}

	// Step 6: Canonical position recomputation + conflict-aware INSERT loop.
	inserted := false
	for i, entry := range entries {
		canonicalPos := basePosition + int64(i) + 1
		entry.Position = canonicalPos

		cmdTag, err := tx.Exec(ctx,
			`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			 ON CONFLICT (token_id, position) DO NOTHING`,
			entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, canonicalPos,
		)
		if err != nil {
			return fmt.Errorf("ApplyTokenChainBatch: insert entry[%d] txID=%s for token %s: %w", i, entry.TransactionID, tokenID, err)
		}

		if cmdTag.RowsAffected() == 0 {
			// Conflict: another row already exists at this position.
			var storedTxID string
			if err := tx.QueryRow(ctx,
				`SELECT transaction_id FROM tokenchain WHERE token_id = $1 AND position = $2`,
				tokenID, canonicalPos,
			).Scan(&storedTxID); err != nil {
				return fmt.Errorf("ApplyTokenChainBatch: resolve conflict at position %d for token %s: %w", canonicalPos, tokenID, err)
			}
			if storedTxID != entry.TransactionID {
				return fmt.Errorf("ApplyTokenChainBatch: chain conflict detected at position %d for token %s: stored txID %s, incoming txID %s", canonicalPos, tokenID, storedTxID, entry.TransactionID)
			}
			// Idempotent: same txID already stored — skip, do not set inserted.
		} else {
			inserted = true
		}
	}

	// Step 7: Conditional index rebuild — only when at least one new row was inserted.
	if inserted {
		if err := w.batchUpsertTokenChainIndex(ctx, tx, []string{tokenID}); err != nil {
			return fmt.Errorf("ApplyTokenChainBatch: upsert tokenchain_index: %w", err)
		}
	}

	// Step 8: Commit.
	return tx.Commit(ctx)
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

func (w *Wallet) GetFullNodeTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	row := w.db.Pool().QueryRow(w.Ctx, `
		SELECT transaction_id, role FROM fullnode_tokenchain WHERE token_id = $1 AND position = $2
		ORDER BY created_at DESC LIMIT 1`, tokenID, height,
	)
	var txID string
	var tokenRoleInTx int16
	if err := row.Scan(&txID, &tokenRoleInTx); err != nil {
		if err == pgx.ErrNoRows {
			return nil, -1, fmt.Errorf("fullnode transaction not found at height %d for token %s", height, tokenID)
		}
		return nil, -1, fmt.Errorf("GetFullNodeTransactionAndRoleAtHeight scan: %w", err)
	}

	tx, err := w.GetTransactionByID(txID)
	if err != nil {
		return nil, -1, fmt.Errorf("GetFullNodeTransactionAndRoleAtHeight transaction details not found for transaction_id: %v, err %w", txID, err)
	}

	return tx, tokenRoleInTx, nil
}

func (w *Wallet) GetTokenChainByTransactionID(transactionID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at
		 FROM tokenchain WHERE transaction_id = $1 ORDER BY position`, transactionID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetTokenChainByTransactionID: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// PersistGenesisTokenRecord atomically inserts a genesis transaction, token, and tokenchain
// entry across five tables (transactions, tokens, tokenchain, tokenchain_index, transaction_units)
// in a single PostgreSQL transaction. Callers must ensure entry.Position == 0,
// entry.Role == mint role, entry.PreviousTransactionID == nil, and txRecord.Signature non-empty.
// Returns an error if invariants are violated. Rollback is automatic on any failure via deferred Rollback.
func (w *Wallet) PersistGenesisTokenRecord(
	tx pgx.Tx,
	dc types.DIDCrypto,
	ps *types.PubSub,
	tokenID string,
	ownerDID string,
	network string,
	epoch int,
) (string, error) {

	// Build Transactions Record
	txInfo := &models.TransactionInfo{
		Initiator: ownerDID,
		Owner:     ownerDID,
		Epoch:     epoch,
		Network:   network,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{
					TokenID:               tokenID,
					PreviousTransactionID: "",
				},
			},
		},
	}

	txInfoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return "", fmt.Errorf("generateTestTokens: failed to serialize transaction info: %w", err)
	}

	signatureBytes, err := dc.Sign(txInfoBytes)
	if err != nil {
		return "", fmt.Errorf("generateTestTokens: failed to sign transaction: %w", err)
	}
	sigStruct := &models.Signature{InitiatorSignature: base64.StdEncoding.EncodeToString(signatureBytes)}

	sigBytes, err := json.Marshal(sigStruct)
	if err != nil {
		return "", fmt.Errorf("generateTestTokens: failed to marshal signature: %w", err)
	}

	txID, err := util.GetTransactionID(txInfo)
	if err != nil {
		return "", fmt.Errorf("generateTestTokens: failed to compute transaction ID: %w", err)
	}

	genesisTx := &models.Transactions{
		ID:        txID,
		Info:      txInfoBytes,
		Signature: json.RawMessage(sigBytes),
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		genesisTx.ID, genesisTx.Info, genesisTx.Signature,
	); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: insert transaction: %w", err)
	}

	// Build Token Record
	mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))
	tokenTypeID := int16(models.GetTokenTypeID(constants.TokenType_RBT))

	token := &models.Token{
		TokenID:        tokenID, // assigned by PersistGenesisTokenRecord
		DID:            ownerDID,
		TokenValue:     rubixmath.OneFloat(),
		TokenStatus:    int16(constants.TokenStatus_Free),
		TransactionID:  txID,
		TokenStateHash: "",
		TokenType:      tokenTypeID,
		LatestPosition: 0,
		LatestRole:     mintRoleID,
	}

	cmdTagToken, err := tx.Exec(w.Ctx,
		`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		 ON CONFLICT (token_id) DO NOTHING`,
		token.TokenID, token.ParentTokenID, token.TokenValue, token.TokenStatus,
		token.DID, token.TransactionID, token.TokenStateHash, token.TokenType,
		token.LatestPosition, token.LatestRole,
	)
	if err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: insert token: %w", err)
	}
	if cmdTagToken.RowsAffected() == 0 {
		return "", fmt.Errorf("PersistGenesisTokenRecord: token %q already exists — duplicate genesis call rejected", token.TokenID)
	}

	// Build Token chain entry
	tokenChainEntry := &models.TokenChain{
		TokenID:               tokenID,
		TransactionID:         txID,
		PreviousTransactionID: nil,
		Role:                  mintRoleID,
		Position:              0,
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (token_id, position) DO NOTHING`,
		tokenChainEntry.TokenID, tokenChainEntry.TransactionID, tokenChainEntry.PreviousTransactionID, tokenChainEntry.Role, tokenChainEntry.Position,
	); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: insert tokenchain: %w", err)
	}

	// Update token chain index
	var index []int32
	if err = tx.QueryRow(w.Ctx,
		`SELECT array_agg(id ORDER BY position) FROM tokenchain WHERE token_id = $1`,
		tokenChainEntry.TokenID,
	).Scan(&index); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: query tokenchain_index: %w", err)
	}
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
		  index = EXCLUDED.index,
		  updated_at = NOW()
	`, tokenChainEntry.TokenID, index); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: upsert tokenchain_index: %w", err)
	}

	// Insert transaction_units record for the genesis initiator.
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, genesisTx.ID, token.DID, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: insert transaction_units: %w", err)
	}

	// Update token_denom table
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO token_denom (did, denom, count, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (did, denom) DO UPDATE SET
		  count = token_denom.count + 1,
		  updated_at = NOW()
	`, token.DID, token.TokenValue, 1); err != nil {
		return "", fmt.Errorf("PersistGenesisTokenRecord: upsert token_denom: %w", err)
	}

	if network != constants.NetworkMode_Localnet {
		if _, err := util.PublishTransaction(ps, txInfo, sigStruct, true, ""); err != nil {
			return "", err
		}
	}

	return txID, tx.Commit(w.Ctx)
}

// TEMP : will be merged with PersistGenesisTokenRecord soon
// prepare FT genesis transaction and process it
func (w *Wallet) FTGenesisTxn(tx pgx.Tx,
	dc types.DIDCrypto,
	ps *types.PubSub,
	did string,
	network string,
	epoch int,
	ftName string,
	startIndex, batchSize int,
	ftValue float64,
	ftRefID int32,
	parentTokens []*models.TokenInfo,
) (txnID string, err error) {

	// prepare FTIDs and details
	txTokensInfo := []*models.TokenInfo{}
	for i := 0; i < batchSize; i++ {
		ftIndex := strconv.Itoa(i + startIndex)
		ftId := strings.Join([]string{ftName, did, ftIndex}, "_")
		txTokensInfo = append(txTokensInfo, &models.TokenInfo{
			TokenID:               ftId,
			PreviousTransactionID: "",
			TokenValue:            ftValue,
			// DID:                   did,
		})
	}

	// prepare transaction info
	txnInfo := &models.TransactionInfo{
		Initiator: did,
		Owner:     did,
		Epoch:     epoch,
		Network:   network,
		Tokens: &models.TransactionTokens{
			FT: txTokensInfo,
		},
		CommittedTokens: parentTokens,
	}

	txInfoBytes, err := models.SerializeTransactionInfo(txnInfo)
	if err != nil {
		return "", fmt.Errorf("FTGenesisTxn: failed to serialize transaction info: %w", err)
	}

	signatureBytes, err := dc.Sign(txInfoBytes)
	if err != nil {
		return "", fmt.Errorf("FTGenesisTxn: failed to sign transaction: %w", err)
	}
	sigStruct := &models.Signature{InitiatorSignature: base64.StdEncoding.EncodeToString(signatureBytes)}

	sigBytes, err := json.Marshal(sigStruct)
	if err != nil {
		return "", fmt.Errorf("FTGenesisTxn: failed to marshal signature: %w", err)
	}

	txnID, err = util.GetTransactionID(txnInfo)
	if err != nil {
		return "", fmt.Errorf("FTGenesisTxn: failed to compute transaction ID: %w", err)
	}

	genesisTx := &models.Transactions{
		ID:        txnID,
		Info:      txInfoBytes,
		Signature: json.RawMessage(sigBytes),
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		genesisTx.ID, genesisTx.Info, genesisTx.Signature,
	); err != nil {
		return "", fmt.Errorf("FTGenesisTxn: insert transaction: %w", err)
	}

	// TODO : update execution_role as per token role or token status
	// Insert transaction_units record for the genesis initiator.
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, txnID, did, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
		return "", fmt.Errorf("FTGenesisTxn: insert transaction_units: %w", err)
	}

	// Build FT Record
	for _, token := range txTokensInfo {
		err := w.InsertGenesisTokenInfo(tx, token, ftRefID, did, txnID, constants.TokenType_FT, constants.TokenRole_Mint)
		if err != nil {
			return "", fmt.Errorf("FTGenesisTxn: failed to update FT info in DB: %w", err)
		}
	}

	for _, parentRBT := range parentTokens {
		// fetch parent token chain indices to get chain height
		// Note: current token chain height = len(tokenChainIndices) - 1 => new height = len(tokenChainIndices)
		var indexLength int
		if err = tx.QueryRow(w.Ctx,
			`SELECT array_length(index, 1)
		 FROM tokenchain_index
		 WHERE token_id = $1`, parentRBT.TokenID,
		).Scan(&indexLength); err != nil {
			// This should NOT return pgx.ErrNoRows because of COALESCE,
			// but handle defensively anyway.
			if errors.Is(err, pgx.ErrNoRows) {
				indexLength = 0
			}
			return "", fmt.Errorf("FTGenesisTxn: query tokenchain_index %w", err)
		}
		// Build Parent Token record
		err = w.UpdateTokenInfo(tx, parentRBT, did, txnID, int64(indexLength), constants.TokenStatus_BurntForFT, constants.TokenType_RBT, constants.TokenRole_Commit)
		if err != nil {
			return "", fmt.Errorf("FTGenesisTxn: failed to update parent RBT %s info in DB: %w", parentRBT.TokenID, err)
		}
	}

	// publish txn
	if _, err = util.PublishTransaction(ps, txnInfo, sigStruct, true, ""); err != nil {
		return "", fmt.Errorf("FTGenesisTxn: publish transaction failed: %w", err)
	}

	return txnID, nil
}

// Genesis token info insertion for all token types
func (w *Wallet) InsertGenesisTokenInfo(tx pgx.Tx, tokenInfo *models.TokenInfo, ftsRefID int32, did, txID, tokenType, tokenRole string) error {
	tokenRoleID := int16(models.GetTokenRoleID(tokenRole))
	tokenTypeID := int16(models.GetTokenTypeID(tokenType))

	// TODO: insert parent token ID for new FTs
	token := &models.Token{
		TokenID:        tokenInfo.TokenID, // assigned by PersistGenesisTokenRecord
		DID:            did,
		TokenValue:     tokenInfo.TokenValue,
		TokenStatus:    int16(constants.TokenStatus_Free),
		TransactionID:  txID,
		TokenStateHash: "",
		TokenType:      tokenTypeID,
		LatestPosition: 0,
		LatestRole:     tokenRoleID,
	}

	cmdTagToken, err := tx.Exec(w.Ctx,
		`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		 ON CONFLICT (token_id) DO NOTHING`,
		token.TokenID, token.ParentTokenID, token.TokenValue, token.TokenStatus,
		token.DID, token.TransactionID, token.TokenStateHash, token.TokenType,
		token.LatestPosition, token.LatestRole,
	)
	if err != nil {
		return fmt.Errorf("InsertGenesisTokenInfo: insert token into tokens table: %w", err)
	}
	if cmdTagToken.RowsAffected() == 0 {
		return fmt.Errorf("InsertGenesisTokenInfo: token %s already exists in tokens table - duplicate genesis call rejected", token.TokenID)
	}

	// updated ft_tokens table with token ID 
	if tokenType == constants.TokenType_FT {
		ftToken := &models.FTTokens{
			TokenID: token.TokenID,
			FTID: ftsRefID,
		}

		cmdTagFTToken, err := tx.Exec(w.Ctx,
			`INSERT INTO ft_tokens (token_id, ft_id, created_at, updated_at)
			 VALUES ($1, $2, NOW(), NOW())
			 ON CONFLICT (token_id) DO NOTHING`,
			ftToken.TokenID, ftToken.FTID,
		)
		if err != nil {
			return fmt.Errorf("InsertGenesisTokenInfo: insert token into ft_tokens table: %w", err)
		}
		if cmdTagFTToken.RowsAffected() == 0 {
			return fmt.Errorf("InsertGenesisTokenInfo: token %s already exists in ft_tokens table - duplicate genesis call rejected", token.TokenID)
		}
	}

	// Build Token chain entry
	tokenChainEntry := &models.TokenChain{
		TokenID:       tokenInfo.TokenID,
		TransactionID: txID,
		Role:          tokenRoleID,
		Position:      0,
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (token_id, position) DO NOTHING`,
		tokenChainEntry.TokenID, tokenChainEntry.TransactionID, tokenChainEntry.PreviousTransactionID, tokenChainEntry.Role, tokenChainEntry.Position,
	); err != nil {
		return fmt.Errorf("InsertGenesisTokenInfo: insert tokenchain: %w", err)
	}

	// Update token chain index
	var index []int32
	if err = tx.QueryRow(w.Ctx,
		`SELECT array_agg(id ORDER BY position) FROM tokenchain WHERE token_id = $1`,
		tokenChainEntry.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("InsertGenesisTokenInfo: query tokenchain: %w", err)
	}
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
		  index = EXCLUDED.index,
		  updated_at = NOW()
	`, tokenChainEntry.TokenID, index); err != nil {
		return fmt.Errorf("InsertGenesisTokenInfo: upsert tokenchain_index: %w", err)
	}

	// Insert transaction_units record for the genesis initiator.
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, txID, token.DID, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
		return fmt.Errorf("InsertGenesisTokenInfo: insert transaction_units: %w", err)
	}

	// Update token_denom table for RBTs
	if tokenType == constants.TokenType_RBT {
		if _, err = tx.Exec(w.Ctx, `
			INSERT INTO token_denom (did, denom, count, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (did, denom) DO UPDATE SET
			  count = token_denom.count + 1,
			  updated_at = NOW()
		`, token.DID, token.TokenValue, 1); err != nil {
			return fmt.Errorf("InsertGenesisTokenInfo: upsert token_denom: %w", err)
		}
	}

	return nil
}

func (w *Wallet) UpdateTokenInfo(tx pgx.Tx, tokenInfo *models.TokenInfo, did, txID string, newTokenChainHeight int64, tokenStatus int, tokenType, tokenRole string) error {
	tokenRoleID := int16(models.GetTokenRoleID(tokenRole))

	// TODO: insert parent token ID for new FTs
	token := &models.Token{
		TokenID:        tokenInfo.TokenID, // assigned by PersistGenesisTokenRecord
		DID:            did,
		TokenStatus:    int16(tokenStatus),
		TransactionID:  txID,
		LatestPosition: newTokenChainHeight,
		LatestRole:     tokenRoleID,
	}
	
	cmdTagToken, err := tx.Exec(w.Ctx,
		`UPDATE tokens SET
            token_status    = $1,
			did 	        = $2,
            transaction_id  = $3,
            latest_position = $4,
            latest_role     = $5,
            updated_at      = NOW()
         WHERE token_id = $6`,
		token.TokenStatus, token.DID, token.TransactionID,
		token.LatestPosition, token.LatestRole, token.TokenID,
	)
	if err != nil {
		return fmt.Errorf("UpdateTokenInfo: update token: %w", err)
	}
	if cmdTagToken.RowsAffected() == 0 {
		return fmt.Errorf("UpdateTokenInfo: token %q not found", token.TokenID)
	}

	// Build Token chain entry
	tokenChainEntry := &models.TokenChain{
		TokenID:               tokenInfo.TokenID,
		TransactionID:         txID,
		PreviousTransactionID: &tokenInfo.PreviousTransactionID,
		Role:                  tokenRoleID,
		Position:              newTokenChainHeight,
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (token_id, position) DO NOTHING`,
		tokenChainEntry.TokenID, tokenChainEntry.TransactionID, tokenChainEntry.PreviousTransactionID, tokenChainEntry.Role, tokenChainEntry.Position,
	); err != nil {
		return fmt.Errorf("UpdateTokenInfo: insert tokenchain: %w", err)
	}

	// Update token chain index
	var index []int32
	if err = tx.QueryRow(w.Ctx,
		`SELECT array_agg(id ORDER BY position) FROM tokenchain WHERE token_id = $1`,
		tokenChainEntry.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("UpdateTokenInfo: query tokenchain: %w", err)
	}
	if _, err = tx.Exec(w.Ctx,
		`UPDATE tokenchain_index SET
            index      = $1,
            updated_at = NOW()
         WHERE token_id = $2`,
		index, tokenChainEntry.TokenID,
	); err != nil {
		return fmt.Errorf("UpdateTokenInfo: update tokenchain_index: %w", err)
	}

	// Update token_denom table for RBTs
	if tokenType == constants.TokenType_RBT {
		if _, err = tx.Exec(w.Ctx, `
			INSERT INTO token_denom (did, denom, count, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (did, denom) DO UPDATE SET
			  count = token_denom.count + 1,
			  updated_at = NOW()
		`, token.DID, token.TokenValue, 1); err != nil {
			return fmt.Errorf("UpdateTokenInfo: upsert token_denom: %w", err)
		}
	}

	return nil
}

// GetTokenchainIndex returns the tokenchain_index row for the given tokenID.
// Returns nil, nil if no row exists.
func (w *Wallet) GetTokenchainIndex(tokenID string) (*models.TokenchainIndex, error) {
	var idx models.TokenchainIndex
	var rawIndex []int32
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, index, created_at, updated_at FROM tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&idx.TokenID, &rawIndex, &idx.CreatedAt, &idx.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetTokenchainIndex: %w", err)
	}
	idx.Index = make([]int, len(rawIndex))
	for i, v := range rawIndex {
		idx.Index[i] = int(v)
	}
	return &idx, nil
}

// GetTokenChainIndices retrieves the index array from tokenchain_index table for a given token.
// The index array represents the chronological order of tokenchain entries for this token.
func (w *Wallet) GetTokenChainIndices(tokenID string) ([]int, error) {
	var indices []int
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT index FROM tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&indices)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GetTokenChainIndices: no tokenchain_index found for token_id %s", tokenID)
		}
		return nil, fmt.Errorf("GetTokenChainIndices: failed to query tokenchain_index: %w", err)
	}

	return indices, nil
}

// GetTransactionsByTokenID retrieves all transactions for a token in chronological order.
func (w *Wallet) GetTransactionsByTokenID(tokenID string) ([]models.Transactions, error) {
	// Step 1: Get the index array from tokenchain_index
	var indices []int
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT index FROM tokenchain_index WHERE token_id = $1`,
		tokenID,
	).Scan(&indices)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("GetTransactionsByTokenID: no tokenchain_index found for token_id %s", tokenID)
		}
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to get indices: %w", err)
	}

	if len(indices) == 0 {
		return []models.Transactions{}, nil
	}

	// Step 2 & 3: Get transactions in exact order using unnest WITH ORDINALITY
	rows, err := w.db.Pool().Query(w.Ctx, `
		SELECT t.id, t.info, t.signature, t.created_at, t.updated_at
		FROM unnest($1::int[]) WITH ORDINALITY AS idx(id, ord)
		JOIN tokenchain tc ON tc.id = idx.id
		JOIN transactions t ON t.id = tc.transaction_id
		ORDER BY idx.ord
	`, indices)
	if err != nil {
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to get transactions: %w", err)
	}

	transactions, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
	if err != nil {
		return nil, fmt.Errorf("GetTransactionsByTokenID: failed to collect rows: %w", err)
	}

	return transactions, nil
}

// GetAllTransactionInfoByTokenId fetches entire token chain, fetches each transaction by transactionId and
// converts into bytes, and returns the chain of ordered transactions with a limit of 100 transactions,
// and the last transaction id of the array in order
func (w *Wallet) GetAllTransactionInfoByTokenId(tokenID string, txnId string) ([]models.Transactions, string, error) {
	txnChain := make([]models.Transactions, 0)
	// get token chain of the token with height limit of 100
	tokenChain, err := w.GetTokenChainByTokenIDAndPrevTxnId(tokenID, txnId)
	if err != nil {
		return nil, "", fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get token chain; error: %v ", err)
	}

	// process each txn in the chain in a loop
	for _, txnInfo := range tokenChain {
		// fetch the txn by txnId
		txn, err := w.GetTransactionByID(txnInfo.TransactionID)
		if err != nil {
			return nil, "", fmt.Errorf("GetAllTransactionsInBytesByTokenId: failed to get transaction by id; error: %v ", err)
		}
		txnChain = append(txnChain, *txn)
	}
	return txnChain, tokenChain[len(tokenChain)-1].TransactionID, nil
}

// GetSmartContractChainByTokenID retrieves all transactions for a smart contract token
// and converts them into TokenChainResponse format with TransactionID, Initiator, Epoch, and Data.
func (w *Wallet) GetSmartContractChainByTokenID(tokenID string) ([]models.TokenChainResponse, error) {
	// Step 1: Validate that the token is a smart contract
	isSC, err := w.IsSmartContract(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: %w", err)
	}
	if !isSC {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: token %s is not a smart contract", tokenID)
	}

	// Step 2: Get all transactions in chronological order
	transactions, err := w.GetTransactionsByTokenID(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetSmartContractChainByTokenID: %w", err)
	}

	// Step 3: Convert transactions to TokenChainResponse using util function
	return util.ConvertToTokenChainResponses(transactions)
}

// GetNFTChainByTokenID retrieves all transactions for an NFT token
// and converts them into TokenChainResponse format with TransactionID, Initiator, Epoch, and Data.
func (w *Wallet) GetNFTChainByTokenID(tokenID string) ([]models.TokenChainResponse, error) {
	// Step 1: Validate that the token is an NFT
	isNFT, err := w.IsNFT(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetNFTChainByTokenID: %w", err)
	}
	if !isNFT {
		return nil, fmt.Errorf("GetNFTChainByTokenID: token %s is not an NFT", tokenID)
	}

	// Step 2: Get all transactions in chronological order
	transactions, err := w.GetTransactionsByTokenID(tokenID)
	if err != nil {
		return nil, fmt.Errorf("GetNFTChainByTokenID: %w", err)
	}

	// Step 3: Convert transactions to TokenChainResponse using util function
	return util.ConvertToTokenChainResponses(transactions)
}
