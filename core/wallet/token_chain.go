package wallet

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	tokenmap "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
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
// entry across five tables (transactions, tokens, tokenchain, tokenchain_index, transaction_units)
// in a single PostgreSQL transaction. Callers must ensure entry.Position == 0,
// entry.Role == mint role, entry.PreviousTransactionID == nil, and txRecord.Signature non-empty.
// Returns an error if invariants are violated. Rollback is automatic on any failure via deferred Rollback.
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

	// Genesis invariant guards — fail fast on invalid inputs.
	mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))
	if txRecord.ID == "" {
		return fmt.Errorf("genesis: empty transaction_id")
	}
	if entry.Position != 0 {
		return fmt.Errorf("PersistGenesisTokenRecord: entry.Position must be 0 for genesis, got %d", entry.Position)
	}
	if entry.Role != mintRoleID {
		return fmt.Errorf("PersistGenesisTokenRecord: entry.Role must be mint (%d) for genesis, got %d", mintRoleID, entry.Role)
	}
	if entry.PreviousTransactionID != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: entry.PreviousTransactionID must be nil for genesis, got %q", *entry.PreviousTransactionID)
	}
	if len(txRecord.Signature) == 0 {
		return fmt.Errorf("PersistGenesisTokenRecord: txRecord.Signature must not be empty for genesis")
	}
	if token.TokenStatus == constants.TokenStatus_Free && len(txRecord.Signature) == 0 {
		return fmt.Errorf("genesis: FREE token must have initiator signature")
	}
	// TODO(phase09-sig): verify signature against txRecord inputs when pubkey available

	// Generate canonical TokenID from global DB counter if the caller left it empty.
	if token.TokenID == "" {
		globalIndex, err := w.GetNextTokenNumber(w.Ctx, tx)
		if err != nil {
			return fmt.Errorf("PersistGenesisTokenRecord: GetNextTokenNumber: %w", err)
		}
		tokenLevel, numInLevel, err := tokenmap.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
		if err != nil {
			return fmt.Errorf("PersistGenesisTokenRecord: GetTokenLevelAndNumberForGlobalIndex(%d): %w", globalIndex, err)
		}
		assignedID := fmt.Sprintf("%d_%d", tokenLevel, numInLevel)
		token.TokenID = assignedID
		entry.TokenID = assignedID
	}
	// Post-assignment invariants (after potential auto-assign).
	if token.TokenID == "" {
		return fmt.Errorf("genesis: empty token_id")
	}
	if token.TransactionID != txRecord.ID {
		return fmt.Errorf("genesis: token %q transaction_id mismatch", token.TokenID)
	}
	if entry.TransactionID != txRecord.ID {
		return fmt.Errorf("genesis: entry transaction_id mismatch for token %q", entry.TokenID)
	}
	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		txRecord.ID, txRecord.Info, txRecord.Signature,
	); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert transaction: %w", err)
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
		return fmt.Errorf("PersistGenesisTokenRecord: insert token: %w", err)
	}
	if cmdTagToken.RowsAffected() == 0 {
		return fmt.Errorf("PersistGenesisTokenRecord: token %q already exists — duplicate genesis call rejected", token.TokenID)
	}

	if _, err = tx.Exec(w.Ctx,
		`INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (token_id, position) DO NOTHING`,
		entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position,
	); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert tokenchain: %w", err)
	}

	// Upsert tokenchain_index atomically within the same transaction.
	var index []int32
	if err = tx.QueryRow(w.Ctx,
		`SELECT array_agg(id ORDER BY position) FROM tokenchain WHERE token_id = $1`,
		entry.TokenID,
	).Scan(&index); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: query tokenchain_index: %w", err)
	}
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (token_id) DO UPDATE SET
		  index = EXCLUDED.index,
		  updated_at = NOW()
	`, entry.TokenID, index); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: upsert tokenchain_index: %w", err)
	}

	// Insert transaction_units record for the genesis initiator.
	if _, err = tx.Exec(w.Ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, txRecord.ID, token.DID, ExecutionRoleInitiator, transactionUnitStatusCommitted); err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: insert transaction_units: %w", err)
	}

	return tx.Commit(w.Ctx)
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
