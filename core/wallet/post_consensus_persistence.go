package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"golang.org/x/crypto/sha3"
)

const (
	DefaultPostConsensusBatchSize = 250

	ExecutionRoleInitiator         = "initiator"
	ExecutionRoleQuorum            = "quorum"
	ExecutionRoleReceiver          = "receiver"
	transactionUnitStatusCommitted = "committed"
)

type PostConsensusPersistenceRequest struct {
	Transaction     *models.Transactions
	TransactionInfo *models.TransactionInfo
	Signature       *models.Signature
	DID             string
	ExecutionRole   string
	AffectedTokens  []string
	TokenChainRows  []models.TokenChain
	TokenStates     []models.Token
}

type PostConsensusPersistenceCoordinator struct {
	wallet    *Wallet
	batchSize int
}

func NewPostConsensusPersistenceCoordinator(w *Wallet) *PostConsensusPersistenceCoordinator {
	return &PostConsensusPersistenceCoordinator{
		wallet:    w,
		batchSize: DefaultPostConsensusBatchSize,
	}
}

func (w *Wallet) PersistPostConsensus(ctx context.Context, req *PostConsensusPersistenceRequest) error {
	return NewPostConsensusPersistenceCoordinator(w).Persist(ctx, req)
}

func (pc *PostConsensusPersistenceCoordinator) Persist(ctx context.Context, req *PostConsensusPersistenceRequest) error {
	if pc == nil || pc.wallet == nil {
		return fmt.Errorf("post-consensus persistence coordinator is not initialized")
	}
	if ctx == nil {
		ctx = pc.wallet.Ctx
	}

	txRecord, err := buildTransactionRecord(req)
	if err != nil {
		return err
	}
	if len(req.TokenChainRows) == 0 || len(req.TokenStates) == 0 {
		derivedTokenChains, derivedTokenStates, derivedAffectedTokens, err := pc.wallet.BuildPersistencePayload(ctx, txRecord.ID, req.TransactionInfo, req.DID, req.ExecutionRole)
		if err != nil {
			return err
		}
		req.TokenChainRows = derivedTokenChains
		req.TokenStates = derivedTokenStates
		if len(req.AffectedTokens) == 0 {
			req.AffectedTokens = derivedAffectedTokens
		}
		pc.wallet.log.Info("derived persistence payload", "transaction_id", txRecord.ID, "did", req.DID, "execution_role", req.ExecutionRole, "affected_tokens", len(req.AffectedTokens))
	} else {
		pc.wallet.log.Info("using provided payload", "transaction_id", txRecord.ID, "did", req.DID, "execution_role", req.ExecutionRole, "affected_tokens", len(req.AffectedTokens))
	}
	if err := validatePostConsensusRequest(req, txRecord.ID); err != nil {
		return err
	}

	tx, err := pc.wallet.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := pc.insertTransaction(ctx, tx, txRecord); err != nil {
		return err
	}
	if err := pc.insertTransactionUnit(ctx, tx, txRecord.ID, req.DID, req.ExecutionRole); err != nil {
		return err
	}
	if err := pc.insertTokenChainRows(ctx, tx, req.TokenChainRows); err != nil {
		return err
	}
	if err := pc.syncTokenChainIndex(ctx, tx, req.AffectedTokens); err != nil {
		return err
	}
	if err := pc.upsertTokenStates(ctx, tx, req.TokenStates); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("post-consensus persistence: commit: %w", err)
	}

	return nil
}

func buildTransactionRecord(req *PostConsensusPersistenceRequest) (*models.Transactions, error) {
	if req == nil {
		return nil, fmt.Errorf("post-consensus persistence: request is nil")
	}

	if req.Transaction != nil {
		record := *req.Transaction
		if record.ID == "" || len(record.Info) == 0 || len(record.Signature) == 0 {
			builtRecord, err := buildTransactionRecordFromPayload(req.TransactionInfo, req.Signature)
			if err != nil {
				return nil, err
			}
			if record.ID == "" {
				record.ID = builtRecord.ID
			}
			if len(record.Info) == 0 {
				record.Info = builtRecord.Info
			}
			if len(record.Signature) == 0 {
				record.Signature = builtRecord.Signature
			}
		}
		if req.TransactionInfo != nil {
			txID, err := ComputeTransactionID(req.TransactionInfo)
			if err != nil {
				return nil, fmt.Errorf("post-consensus persistence: compute transaction id: %w", err)
			}
			if record.ID != txID {
				return nil, fmt.Errorf("post-consensus persistence: transaction id mismatch")
			}
		}
		return &record, nil
	}

	return buildTransactionRecordFromPayload(req.TransactionInfo, req.Signature)
}

func buildTransactionRecordFromPayload(txInfo *models.TransactionInfo, signature *models.Signature) (*models.Transactions, error) {
	if txInfo == nil {
		return nil, fmt.Errorf("post-consensus persistence: transaction info is required")
	}
	if signature == nil {
		return nil, fmt.Errorf("post-consensus persistence: transaction signature is required")
	}

	txID, err := ComputeTransactionID(txInfo)
	if err != nil {
		return nil, fmt.Errorf("post-consensus persistence: compute transaction id: %w", err)
	}

	infoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return nil, fmt.Errorf("post-consensus persistence: marshal transaction info: %w", err)
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		return nil, fmt.Errorf("post-consensus persistence: marshal transaction signature: %w", err)
	}

	return &models.Transactions{
		ID:        txID,
		Info:      infoBytes,
		Signature: signatureBytes,
	}, nil
}

// ComputeTransactionID computes a deterministic hex-encoded SHA3-256 transaction ID from txInfo
// using models.SerializeTransactionInfo as the canonical encoding.
// Returns hex string (not raw bytes) for DB and cross-node compatibility.
func ComputeTransactionID(txInfo *models.TransactionInfo) (string, error) {
	txInfoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return "", err
	}

	hash := sha3.Sum256(txInfoBytes)
	return hex.EncodeToString(hash[:]), nil
}

func validatePostConsensusRequest(req *PostConsensusPersistenceRequest, transactionID string) error {
	if req == nil {
		return fmt.Errorf("post-consensus persistence: request is nil")
	}
	if transactionID == "" {
		return fmt.Errorf("post-consensus persistence: transaction id is required")
	}
	if req.DID == "" {
		return fmt.Errorf("post-consensus persistence: did is required")
	}
	if !isValidExecutionRole(req.ExecutionRole) {
		return fmt.Errorf("post-consensus persistence: invalid execution role %q", req.ExecutionRole)
	}
	if len(req.AffectedTokens) == 0 {
		return fmt.Errorf("post-consensus persistence: affected tokens are required")
	}
	if len(req.TokenChainRows) == 0 {
		return fmt.Errorf("post-consensus persistence: tokenchain rows are required")
	}
	if len(req.TokenStates) == 0 {
		return fmt.Errorf("post-consensus persistence: token states are required")
	}

	affected := make(map[string]struct{}, len(req.AffectedTokens))
	for _, tokenID := range req.AffectedTokens {
		if tokenID == "" {
			return fmt.Errorf("post-consensus persistence: affected token id is empty")
		}
		if _, exists := affected[tokenID]; exists {
			return fmt.Errorf("post-consensus persistence: duplicate affected token %q", tokenID)
		}
		affected[tokenID] = struct{}{}
	}

	tokenChainByToken := make(map[string]models.TokenChain, len(req.TokenChainRows))
	for _, row := range req.TokenChainRows {
		if row.TokenID == "" {
			return fmt.Errorf("post-consensus persistence: tokenchain row is missing token id")
		}
		if _, exists := affected[row.TokenID]; !exists {
			return fmt.Errorf("post-consensus persistence: tokenchain row token %q is not part of the transaction unit", row.TokenID)
		}
		if row.TransactionID != transactionID {
			return fmt.Errorf("post-consensus persistence: tokenchain row transaction id mismatch for token %q", row.TokenID)
		}
		if row.Position < 0 {
			return fmt.Errorf("post-consensus persistence: tokenchain row position must be non-negative for token %q", row.TokenID)
		}
		if row.Position == 0 && row.PreviousTransactionID != nil && *row.PreviousTransactionID != "" {
			return fmt.Errorf("post-consensus persistence: genesis tokenchain row must not have previous transaction id for token %q", row.TokenID)
		}
		if row.Position > 0 && (row.PreviousTransactionID == nil || *row.PreviousTransactionID == "") {
			return fmt.Errorf("post-consensus persistence: tokenchain row must have previous transaction id for token %q", row.TokenID)
		}
		if _, exists := tokenChainByToken[row.TokenID]; exists {
			return fmt.Errorf("post-consensus persistence: multiple tokenchain rows provided for token %q", row.TokenID)
		}
		tokenChainByToken[row.TokenID] = row
	}

	tokenStateByToken := make(map[string]models.Token, len(req.TokenStates))
	for _, tokenState := range req.TokenStates {
		if tokenState.TokenID == "" {
			return fmt.Errorf("post-consensus persistence: token state is missing token id")
		}
		if _, exists := affected[tokenState.TokenID]; !exists {
			return fmt.Errorf("post-consensus persistence: token state token %q is not part of the transaction unit", tokenState.TokenID)
		}
		if tokenState.TransactionID != transactionID {
			return fmt.Errorf("post-consensus persistence: token state transaction id mismatch for token %q", tokenState.TokenID)
		}
		if _, exists := tokenStateByToken[tokenState.TokenID]; exists {
			return fmt.Errorf("post-consensus persistence: multiple token states provided for token %q", tokenState.TokenID)
		}

		chainRow, exists := tokenChainByToken[tokenState.TokenID]
		if !exists {
			return fmt.Errorf("post-consensus persistence: missing tokenchain row for token %q", tokenState.TokenID)
		}
		if tokenState.LatestPosition != chainRow.Position {
			return fmt.Errorf("post-consensus persistence: token state position mismatch for token %q", tokenState.TokenID)
		}
		if tokenState.LatestRole != chainRow.Role {
			return fmt.Errorf("post-consensus persistence: token state role mismatch for token %q", tokenState.TokenID)
		}

		tokenStateByToken[tokenState.TokenID] = tokenState
	}

	for tokenID := range affected {
		if _, exists := tokenChainByToken[tokenID]; !exists {
			return fmt.Errorf("post-consensus persistence: missing tokenchain row for token %q", tokenID)
		}
		if _, exists := tokenStateByToken[tokenID]; !exists {
			return fmt.Errorf("post-consensus persistence: missing token state for token %q", tokenID)
		}
	}

	return nil
}

func isValidExecutionRole(role string) bool {
	switch role {
	case ExecutionRoleInitiator, ExecutionRoleQuorum, ExecutionRoleReceiver:
		return true
	default:
		return false
	}
}

func (pc *PostConsensusPersistenceCoordinator) insertTransaction(ctx context.Context, tx pgx.Tx, record *models.Transactions) error {
	cmdTag, err := tx.Exec(ctx, `
		INSERT INTO transactions (id, info, signature, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, record.ID, record.Info, record.Signature)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: insert transaction: %w", err)
	}
	if cmdTag.RowsAffected() > 0 {
		return nil
	}

	var existingInfo []byte
	var existingSignature []byte
	if err := tx.QueryRow(ctx,
		`SELECT info, signature FROM transactions WHERE id = $1`,
		record.ID,
	).Scan(&existingInfo, &existingSignature); err != nil {
		return fmt.Errorf("post-consensus persistence: read existing transaction: %w", err)
	}
	if !bytes.Equal(existingInfo, record.Info) || !bytes.Equal(existingSignature, record.Signature) {
		return fmt.Errorf("post-consensus persistence: transaction payload mismatch for id %q", record.ID)
	}

	return nil
}

func (pc *PostConsensusPersistenceCoordinator) insertTransactionUnit(ctx context.Context, tx pgx.Tx, transactionID, did, executionRole string) error {
	cmdTag, err := tx.Exec(ctx, `
		INSERT INTO transaction_units (transaction_id, did, execution_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (transaction_id, did) DO NOTHING
	`, transactionID, did, executionRole, transactionUnitStatusCommitted)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: insert transaction unit: %w", err)
	}
	if cmdTag.RowsAffected() > 0 {
		return nil
	}

	var existingRole string
	var existingStatus string
	if err := tx.QueryRow(ctx,
		`SELECT execution_role, status FROM transaction_units WHERE transaction_id = $1 AND did = $2`,
		transactionID, did,
	).Scan(&existingRole, &existingStatus); err != nil {
		return fmt.Errorf("post-consensus persistence: read existing transaction unit: %w", err)
	}
	if existingRole != executionRole || existingStatus != transactionUnitStatusCommitted {
		return fmt.Errorf("post-consensus persistence: transaction unit conflict for transaction %q and did %q", transactionID, did)
	}

	return nil
}

func (pc *PostConsensusPersistenceCoordinator) insertTokenChainRows(ctx context.Context, tx pgx.Tx, rows []models.TokenChain) error {
	for start := 0; start < len(rows); start += pc.batchSize {
		end := start + pc.batchSize
		if end > len(rows) {
			end = len(rows)
		}

		chunk := rows[start:end]
		placeholders := make([]string, 0, len(chunk))
		args := make([]interface{}, 0, len(chunk)*5)
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
				return fmt.Errorf("post-consensus persistence: insert tokenchain rows: %w", err)
		}
	}

	return nil
}

func (pc *PostConsensusPersistenceCoordinator) syncTokenChainIndex(ctx context.Context, tx pgx.Tx, tokenIDs []string) error {
	rows, err := tx.Query(ctx, `
		SELECT token_id, array_agg(id ORDER BY position)
		FROM tokenchain
		WHERE token_id = ANY($1::text[])
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: read tokenchain index: %w", err)
	}
	defer rows.Close()

	type tokenchainIndexRow struct {
		tokenID string
		index   []int32
	}

	indexRows := make([]tokenchainIndexRow, 0, len(tokenIDs))
	for rows.Next() {
		var row tokenchainIndexRow
		if err := rows.Scan(&row.tokenID, &row.index); err != nil {
			return fmt.Errorf("post-consensus persistence: scan tokenchain index: %w", err)
		}
		indexRows = append(indexRows, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("post-consensus persistence: stream tokenchain index: %w", err)
	}
	if len(indexRows) == 0 {
		return nil
	}

	placeholders := make([]string, 0, len(indexRows))
	args := make([]interface{}, 0, len(indexRows)*2)
	for i, row := range indexRows {
		offset := i*2 + 1
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, NOW(), NOW())", offset, offset+1))
		args = append(args, row.tokenID, row.index)
	}

	query := `
		INSERT INTO tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ` + strings.Join(placeholders, ",") + `
		ON CONFLICT (token_id) DO UPDATE SET
			index = EXCLUDED.index,
			updated_at = NOW()
	`
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("post-consensus persistence: upsert tokenchain index: %w", err)
	}

	return nil
}

func (pc *PostConsensusPersistenceCoordinator) upsertTokenStates(ctx context.Context, tx pgx.Tx, tokenStates []models.Token) error {
	for start := 0; start < len(tokenStates); start += pc.batchSize {
		end := start + pc.batchSize
		if end > len(tokenStates) {
			end = len(tokenStates)
		}

		chunk := tokenStates[start:end]
		placeholders := make([]string, 0, len(chunk))
		args := make([]interface{}, 0, len(chunk)*10)
		for i, tokenState := range chunk {
			offset := i*10 + 1
			placeholders = append(placeholders,
				fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW(), NOW())",
					offset, offset+1, offset+2, offset+3, offset+4,
					offset+5, offset+6, offset+7, offset+8, offset+9,
				),
			)
			args = append(args,
				tokenState.TokenID,
				tokenState.ParentTokenID,
				tokenState.TokenValue,
				tokenState.TokenStatus,
				tokenState.DID,
				tokenState.TransactionID,
				tokenState.TokenStateHash,
				tokenState.TokenType,
				tokenState.LatestPosition,
				tokenState.LatestRole,
			)
		}

		query := `
			INSERT INTO tokens (
				token_id, parent_token_id, token_value, token_status, did, transaction_id,
				token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
			)
			VALUES ` + strings.Join(placeholders, ",") + `
			ON CONFLICT (token_id) DO UPDATE SET
				parent_token_id = EXCLUDED.parent_token_id,
				token_value = EXCLUDED.token_value,
				token_status = EXCLUDED.token_status,
				did = EXCLUDED.did,
				transaction_id = EXCLUDED.transaction_id,
				token_state_hash = EXCLUDED.token_state_hash,
				token_type = EXCLUDED.token_type,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("post-consensus persistence: upsert token states: %w", err)
		}
	}

	return nil
}
