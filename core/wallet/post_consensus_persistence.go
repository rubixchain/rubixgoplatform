package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	DefaultPostConsensusBatchSize = 250

	ExecutionRoleInitiator         = "initiator"
	ExecutionRoleQuorum            = "quorum"
	ExecutionRoleReceiver          = "receiver"
	transactionUnitStatusCommitted = "committed"
)

type PostConsensusPersistenceRequest struct {
	Transaction               *models.Transactions
	TransactionInfo           *models.TransactionInfo
	Signature                 *models.Signature
	DID                       string
	ExecutionRole             string
	AffectedTokens            []string
	TokenChainRows            []models.TokenChain
	TokenStates               []models.Token
	SkipSignatureVerification bool
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

	if req.TransactionInfo == nil {
		return fmt.Errorf("post-consensus persistence: transaction info is required for transaction id binding")
	}
	if req.TransactionInfo.Owner == "" {
		return fmt.Errorf("post-consensus persistence: transaction owner is required")
	}

	// Check if the transaction is local
	isLocalTransfer, err := pc.wallet.IsLocalDID(req.TransactionInfo.Owner)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: failed to check if owner DID is local: %w", err)
	}

	// Ideally local transfer will only happen on the initiator side.
	// The following prevents any invariant state where a non-initiator node mistakenly
	// treats a transfer as local and thus fails to update token status to Transferred, 
	// which would cause balance inconsistencies and other issues.
	isLocalTransfer = isLocalTransfer && (req.ExecutionRole == ExecutionRoleInitiator)

	if len(req.TokenChainRows) == 0 || len(req.TokenStates) == 0 {
		derivedTokenChains, derivedTokenStates, derivedAffectedTokens, err := pc.wallet.BuildPersistencePayload(ctx, txRecord.ID, req.TransactionInfo, req.DID, req.ExecutionRole, isLocalTransfer)
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
	if pc.wallet.didDir == "" {
		return fmt.Errorf("transfer: signature verification not configured")
	}
	// Signature verification — must run BEFORE BeginTx so an invalid request
	// never enters a transaction. Requires w.didDir set via SetDidDir().
	if pc.wallet.didDir != "" && !req.SkipSignatureVerification {
		var sig models.Signature
		if err := json.Unmarshal(txRecord.Signature, &sig); err != nil {
			return fmt.Errorf("transfer: invalid signature format")
		}
		if sig.InitiatorSignature == "" {
			return fmt.Errorf("transfer: missing initiator signature")
		}
		dc := did.InitDIDLiteWithPassword(req.DID, pc.wallet.didDir, "")
		if err := util.VerifySignature(dc, req.TransactionInfo, sig.InitiatorSignature); err != nil {
			return fmt.Errorf("transfer: invalid initiator signature")
		}
	}

	tx, err := pc.wallet.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("post-consensus persistence: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := pc.validateTransferChainContinuity(ctx, tx, req); err != nil {
		return err
	}

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
	if err := pc.upsertTokenDenomDeltas(ctx, tx, req.DID, req.TokenStates, req.ExecutionRole, isLocalTransfer, req.TransactionInfo.Owner); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("post-consensus persistence: commit: %w", err)
	}

	return nil
}

func buildTransactionRecord(req *PostConsensusPersistenceRequest) (*models.Transactions, error) {
	if req == nil {
		return nil, fmt.Errorf("transfer: request is nil")
	}

	if req.Transaction != nil {
		record := *req.Transaction
		if record.ID == "" || len(record.Info) == 0 || len(record.Signature) == 0 {
			builtRecord, err := BuildTransactionRecordFromPayload(req.TransactionInfo, req.Signature)
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
		if req.TransactionInfo == nil {
			return nil, fmt.Errorf("transfer: transaction info required for transaction id binding")
		}
		txID, err := util.GetTransactionID(req.TransactionInfo)
		if err != nil {
			return nil, fmt.Errorf("transfer: failed to recompute transaction id: %v", err)
		}
		if record.ID != txID {
			return nil, fmt.Errorf("transfer: transaction id mismatch: request claims %q, payload computes %q", record.ID, txID)
		}
		return &record, nil
	}

	return BuildTransactionRecordFromPayload(req.TransactionInfo, req.Signature)
}

func BuildTransactionRecordFromPayload(txInfo *models.TransactionInfo, signature *models.Signature) (*models.Transactions, error) {
	if txInfo == nil {
		return nil, fmt.Errorf("post-consensus persistence: transaction info is required")
	}
	if signature == nil {
		return nil, fmt.Errorf("post-consensus persistence: transaction signature is required")
	}

	txID, err := util.GetTransactionID(txInfo)
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

// validateTransferChainContinuity validates that each non-genesis TokenChain row in req
// references a token that exists, belongs to req.DID, is Free, and continues the chain
// with the correct position and previous_transaction_id. All reads use the provided pgx.Tx
// (FOR UPDATE) to ensure they are consistent with the pending writes.
func (pc *PostConsensusPersistenceCoordinator) validateTransferChainContinuity(ctx context.Context, tx pgx.Tx, req *PostConsensusPersistenceRequest) error {
	if req == nil {
		return fmt.Errorf("transfer: empty transaction_id")
	}

	// Build a set of token IDs declared in the signed txInfo payload.
	// Every token in the request must be present in txInfo (no payload tampering).
	// This check is pure struct-level — no DB access.
	txInfoTokenSet := make(map[string]bool)
	if req.TransactionInfo != nil && req.TransactionInfo.Tokens != nil {
		for _, t := range req.TransactionInfo.Tokens.RBT {
			if t != nil {
				txInfoTokenSet[t.TokenID] = true
			}
		}
		for _, t := range req.TransactionInfo.Tokens.NFT {
			if t != nil {
				txInfoTokenSet[t.TokenID] = true
			}
		}
		for _, t := range req.TransactionInfo.Tokens.FT {
			if t != nil {
				txInfoTokenSet[t.TokenID] = true
			}
		}
		for _, t := range req.TransactionInfo.Tokens.SmartContract {
			if t != nil {
				txInfoTokenSet[t.TokenID] = true
			}
		}
	}

	for _, row := range req.TokenChainRows {
		if row.Position == 0 {
			// Genesis rows are validated by genesis-specific paths.
			continue
		}
		if row.TokenID == "" {
			return fmt.Errorf("transfer: empty token_id")
		}
		if len(txInfoTokenSet) > 0 && !txInfoTokenSet[row.TokenID] {
			return fmt.Errorf("transfer: token %q not present in transaction payload", row.TokenID)
		}

		var dbDID string
		var dbStatus int16
		var dbTransactionID string
		var dbLatestPosition int64

		err := tx.QueryRow(ctx, `
			SELECT did, token_status, transaction_id, latest_position
			FROM tokens
			WHERE token_id = $1
			FOR UPDATE
		`, row.TokenID).Scan(&dbDID, &dbStatus, &dbTransactionID, &dbLatestPosition)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("transfer: token %q does not exist", row.TokenID)
			}
			return fmt.Errorf("transfer: query token %q: %w", row.TokenID, err)
		}

		/*
			if dbDID != req.DID {
				return fmt.Errorf("transfer: token %q not owned by %s", row.TokenID, req.DID)
			}
		*/

		// Initiator/Quorum: token must be Free or Locked (Locked by LockTokensForSplit before consensus).
		// Receiver: token must be Free, Locked, or Transferred. Transferred covers the case where
		// this token was previously sent out by this node and is now returning. Terminal and error
		// states (Burnt, Orphaned, ChainSyncIssue, BeingDoubleSpent) are explicitly rejected.
		if req.ExecutionRole != ExecutionRoleReceiver {
			// Initiator/Quorum: token must be Free or Locked (Locked by LockTokensForSplit before consensus).
			if dbStatus != int16(constants.TokenStatus_Free) && dbStatus != int16(constants.TokenStatus_Locked) {
				return fmt.Errorf("transfer: token %q is not FREE or LOCKED (status=%d), cannot persist", row.TokenID, dbStatus)
			}
		} else {
			// Receiver: also permit Transferred — token was sent out and is now returning to this node.
			// Terminal and error states (Burnt, Orphaned, ChainSyncIssue, BeingDoubleSpent) are rejected.
			if dbStatus != int16(constants.TokenStatus_Free) &&
				dbStatus != int16(constants.TokenStatus_Locked) &&
				dbStatus != int16(constants.TokenStatus_Transferred) {
				return fmt.Errorf("transfer: token %q has unexpected status %d for receiver (want Free, Locked, or Transferred)", row.TokenID, dbStatus)
			}
		}

		if row.PreviousTransactionID != nil && *row.PreviousTransactionID != dbTransactionID {
			return fmt.Errorf("transfer: token %q previous_transaction_id mismatch", row.TokenID)
		}

		if row.Position != dbLatestPosition+1 {
			return fmt.Errorf("transfer: token %q position must be latest+1, got %d want %d", row.TokenID, row.Position, dbLatestPosition+1)
		}
	}

	return nil
}

func (pc *PostConsensusPersistenceCoordinator) insertTransaction(ctx context.Context, tx pgx.Tx, record *models.Transactions) error {
	// PersistPostConsensus carries the full canonical signature (all quorum
	// entries assembled by the initiator). PledgeV2 may have already inserted
	// a transactions row with a partial signature (single quorum entry) on the
	// same node. Using DO UPDATE SET ensures this function always writes the
	// authoritative payload, silently upgrading any partial row from PledgeV2.
	if _, err := tx.Exec(ctx, `
		INSERT INTO transactions (id, info, signature, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			info = EXCLUDED.info,
			signature = EXCLUDED.signature,
			updated_at = NOW()
	`, record.ID, record.Info, record.Signature); err != nil {
		return fmt.Errorf("post-consensus persistence: insert transaction: %w", err)
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
		end := min(start+pc.batchSize, len(rows))

		chunk := rows[start:end]
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
	args := make([]any, 0, len(indexRows)*2)
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
		end := min(start+pc.batchSize, len(tokenStates))

		chunk := tokenStates[start:end]
		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*10)
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

// upsertTokenDenomDeltas updates the token_denom table within the same DB transaction.
// For ExecutionRoleReceiver: increment denom count for each received token.
// For ExecutionRoleInitiator: decrement denom count for each sent token.
// For ExecutionRoleQuorum (pledge): no change — pledged tokens are tracked via token_status, not denom.
func (pc *PostConsensusPersistenceCoordinator) upsertTokenDenomDeltas(ctx context.Context, tx pgx.Tx, did string, tokenStates []models.Token, executionRole string, isLocalTransfer bool, ownerDID string) error {
	denomDelta := make(map[float64]int64)
	for _, state := range tokenStates {
		if state.TokenValue == 0 {
			continue
		}
		switch executionRole {
		case ExecutionRoleReceiver:
			denomDelta[state.TokenValue]++
		case ExecutionRoleInitiator:
			denomDelta[state.TokenValue]--
		}
	}
	if len(denomDelta) == 0 {
		return nil
	}
	for denom, delta := range denomDelta {
		if delta == 0 {
			continue
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO token_denom (did, denom, count, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (did, denom) DO UPDATE SET
				count = token_denom.count + $3,
				updated_at = NOW()
		`, did, denom, delta)
		if err != nil {
			return fmt.Errorf("post-consensus persistence: upsert token_denom (did=%s denom=%v delta=%d): %w", did, denom, delta, err)
		}
	}

	// TODO: refactor to avoid redudant code
	if isLocalTransfer {
		ownerDenomDelta := make(map[float64]int64)

		for _, state := range tokenStates {
			if state.TokenValue == 0 {
				continue
			}

			ownerDenomDelta[state.TokenValue]++
		}

		for denom, delta := range ownerDenomDelta {
			if delta == 0 {
				continue
			}

			_, err := tx.Exec(ctx, `
				INSERT INTO token_denom (did, denom, count, created_at, updated_at)
				VALUES ($1, $2, $3, NOW(), NOW())
				ON CONFLICT (did, denom) DO UPDATE SET
					count = token_denom.count + $3,
					updated_at = NOW()
			`, ownerDID, denom, delta)
			if err != nil {
				return fmt.Errorf("post-consensus persistence: upsert token_denom (did=%s denom=%v delta=%d): %w", ownerDID, denom, delta, err)
			}
		}
	}

	return nil
}
