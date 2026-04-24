package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

type FullNodePersistenceRequest struct {
	Transaction     *models.Transactions
	TransactionInfo *models.TransactionInfo
}

type fullNodeTokenInput struct {
	tokenInfo *models.TokenInfo
	tokenType string
	roleName  string
}

type fullNodeTokenState struct {
	tokenID        string
	parentTokenID  pgtype.Text
	tokenValue     float64
	tokenStatus    int16
	did            string
	transactionID  string
	tokenStateHash string
	latestPosition int64
	latestRole     int16
	exists         bool
}

func (w *Wallet) PersistFullNodeTransaction(ctx context.Context, req *FullNodePersistenceRequest) error {
	if w == nil {
		return fmt.Errorf("fullnode persistence: wallet is nil")
	}
	if req == nil || req.Transaction == nil || req.TransactionInfo == nil {
		return fmt.Errorf("fullnode persistence: request, transaction and transaction info are required")
	}
	if req.Transaction.ID == "" {
		return fmt.Errorf("fullnode persistence: transaction id is required")
	}
	if ctx == nil {
		ctx = w.Ctx
	}

	inputs, affectedTokenIDs, err := collectFullNodeTokenInputs(req.TransactionInfo)
	if err != nil {
		return err
	}
	w.log.Debug("PersistFullNodeTransaction: inputs", inputs, "affectedTokenIDs", affectedTokenIDs)
	if len(inputs) == 0 {
		return fmt.Errorf("fullnode persistence: no tokens found in transaction payload")
	}

	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("fullnode persistence: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := w.insertFullNodeTransaction(ctx, tx, req.Transaction); err != nil {
		return err
	}

	tokenChainRows := make([]models.TokenChain, 0, len(inputs))
	for _, input := range inputs {
		roleName := input.roleName
		if roleName == constants.TokenRole_Transfer && input.tokenInfo.PreviousTransactionID == "" {
			roleName = constants.TokenRole_Mint
		}
		roleID := models.GetTokenRoleID(roleName)
		if roleID <= 0 {
			return fmt.Errorf("fullnode persistence: unsupported token role %q for token %q", roleName, input.tokenInfo.TokenID)
		}

		tokenState, err := w.readFullNodeTokenStateTx(ctx, tx, input.tokenType, input.tokenInfo.TokenID)
		if err != nil {
			return err
		}

		latestRow, err := w.readLatestFullNodeTokenChainRowTx(ctx, tx, input.tokenInfo.TokenID)
		if err != nil {
			return err
		}

		chainRow, position := buildFullNodeTokenChainRow(req.Transaction.ID, latestRow, input.tokenInfo.PreviousTransactionID, int16(roleID), input.tokenInfo.TokenID)
		tokenChainRows = append(tokenChainRows, chainRow)

		tokenState = deriveFullNodeTokenState(tokenState, req.TransactionInfo, input, req.Transaction.ID, int16(roleID), position)
		if err := w.upsertFullNodeTokenStateTx(ctx, tx, input.tokenType, tokenState); err != nil {
			return err
		}
	}

	if err := w.insertFullNodeTokenChainRows(ctx, tx, tokenChainRows); err != nil {
		return err
	}
	if err := w.syncFullNodeTokenChainIndex(ctx, tx, affectedTokenIDs); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("fullnode persistence: commit: %w", err)
	}
	return nil
}

func collectFullNodeTokenInputs(txInfo *models.TransactionInfo) ([]fullNodeTokenInput, []string, error) {
	if txInfo == nil {
		return nil, nil, fmt.Errorf("fullnode persistence: transaction info is nil")
	}
	seen := make(map[string]struct{})
	inputs := make([]fullNodeTokenInput, 0)
	affected := make([]string, 0)

	appendInputs := func(tokens []*models.TokenInfo, tokenType string, roleName string) error {
		for _, token := range tokens {
			if token == nil {
				return fmt.Errorf("fullnode persistence: token payload is nil")
			}
			if token.TokenID == "" {
				return fmt.Errorf("fullnode persistence: token id is empty")
			}
			if _, exists := seen[token.TokenID]; exists {
				return fmt.Errorf("fullnode persistence: duplicate token %q in transaction payload", token.TokenID)
			}
			seen[token.TokenID] = struct{}{}
			inputs = append(inputs, fullNodeTokenInput{
				tokenInfo: token,
				tokenType: tokenType,
				roleName:  roleName,
			})
			affected = append(affected, token.TokenID)
		}
		return nil
	}

	if txInfo.Tokens != nil {
		if err := appendInputs(txInfo.Tokens.RBT, constants.TokenType_RBT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.FT, constants.TokenType_FT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.NFT, constants.TokenType_NFT, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
		if err := appendInputs(txInfo.Tokens.SmartContract, constants.TokenType_SmartContract, constants.TokenRole_Transfer); err != nil {
			return nil, nil, err
		}
	}
	if err := appendInputs(txInfo.CommittedTokens, constants.TokenType_RBT, constants.TokenRole_Commit); err != nil {
		return nil, nil, err
	}

	return inputs, affected, nil
}

func (w *Wallet) insertFullNodeTransaction(ctx context.Context, tx pgx.Tx, transaction *models.Transactions) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fullnode_transactions (id, info, signature, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, transaction.ID, transaction.Info, transaction.Signature)
	if err != nil {
		return fmt.Errorf("fullnode persistence: insert fullnode transaction: %w", err)
	}
	w.log.Debug("insertFullNodeTransaction: Transactions inserted successfully")
	return nil
}

func (w *Wallet) readLatestFullNodeTokenChainRowTx(ctx context.Context, tx pgx.Tx, tokenID string) (*models.TokenChain, error) {
	var row models.TokenChain
	err := tx.QueryRow(ctx, `
		SELECT id, token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at
		FROM fullnode_tokenchain
		WHERE token_id = $1
		ORDER BY position DESC
		LIMIT 1
	`, tokenID).Scan(
		&row.ID, &row.TokenID, &row.TransactionID, &row.PreviousTransactionID, &row.Role, &row.Position, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("fullnode persistence: read latest tokenchain row for token %q: %w", tokenID, err)
	}
	return &row, nil
}

func buildFullNodeTokenChainRow(transactionID string, latestRow *models.TokenChain, previousTransactionID string, roleID int16, tokenID string) (models.TokenChain, int64) {
	var position int64
	var prevTxID *string
	if latestRow != nil {
		position = latestRow.Position + 1
		prev := latestRow.TransactionID
		prevTxID = &prev
	} else {
		if previousTransactionID != "" {
			position = 1
			prev := previousTransactionID
			prevTxID = &prev
		} else {
			position = 0
			prev := transactionID
			prevTxID = &prev
		}
	}
	return models.TokenChain{
		TokenID:               tokenID,
		TransactionID:         transactionID,
		PreviousTransactionID: prevTxID,
		Role:                  roleID,
		Position:              position,
	}, position
}

func deriveFullNodeTokenState(existing fullNodeTokenState, txInfo *models.TransactionInfo, input fullNodeTokenInput, transactionID string, roleID int16, position int64) fullNodeTokenState {
	state := existing
	if !state.exists {
		state.tokenID = input.tokenInfo.TokenID
		state.tokenStatus = int16(constants.TokenStatus_Free)
	}
	if input.tokenInfo.TokenValue > 0 {
		state.tokenValue = input.tokenInfo.TokenValue
	} else {
		//In tokenInfo if token value is passed as 0, it can be NFT or RBT(whose value passed as 0),
		//In any other case token value passed as non 0,
		//So if the tokenID is starting with not starting with "Qm", assume that it as RBT and compute the tokenValue from the tokenID
		if !strings.HasPrefix(input.tokenInfo.TokenID, "Qm") {
			derivedValue, err := util.GetTokenValueFromTokenID(input.tokenInfo.TokenID)
			if err != nil {
				return fullNodeTokenState{}
			}
			state.tokenValue = derivedValue
		}
	}

	if input.tokenInfo.Data != "" {
		state.tokenStateHash = input.tokenInfo.Data
	}
	if txInfo.Owner != "" {
		state.did = txInfo.Owner
	}
	state.transactionID = transactionID
	state.latestPosition = position
	state.latestRole = roleID
	state.exists = true
	return state
}

func (w *Wallet) readFullNodeTokenStateTx(ctx context.Context, tx pgx.Tx, tokenType string, tokenID string) (fullNodeTokenState, error) {
	switch tokenType {
	case constants.TokenType_RBT:
		var state fullNodeTokenState
		err := tx.QueryRow(ctx, `
			SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
			       token_state_hash, latest_position, latest_role
			FROM fullnode_rbt
			WHERE token_id = $1
		`, tokenID).Scan(
			&state.tokenID, &state.parentTokenID, &state.tokenValue, &state.tokenStatus, &state.did,
			&state.transactionID, &state.tokenStateHash, &state.latestPosition, &state.latestRole,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fullNodeTokenState{}, nil
			}
			return fullNodeTokenState{}, fmt.Errorf("fullnode persistence: read fullnode_rbt token %q: %w", tokenID, err)
		}
		state.exists = true
		return state, nil
	case constants.TokenType_FT:
		var state fullNodeTokenState
		err := tx.QueryRow(ctx, `
			SELECT token_id, token_value, token_status, did, transaction_id,
			       token_state_hash, latest_position, latest_role
			FROM fullnode_ft
			WHERE token_id = $1
		`, tokenID).Scan(
			&state.tokenID, &state.tokenValue, &state.tokenStatus, &state.did,
			&state.transactionID, &state.tokenStateHash, &state.latestPosition, &state.latestRole,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fullNodeTokenState{}, nil
			}
			return fullNodeTokenState{}, fmt.Errorf("fullnode persistence: read fullnode_ft token %q: %w", tokenID, err)
		}
		state.exists = true
		return state, nil
	case constants.TokenType_NFT:
		var state fullNodeTokenState
		err := tx.QueryRow(ctx, `
			SELECT token_id, token_value, token_status, did, transaction_id,
			       token_state_hash, latest_position, latest_role
			FROM fullnode_nft
			WHERE token_id = $1
		`, tokenID).Scan(
			&state.tokenID, &state.tokenValue, &state.tokenStatus, &state.did,
			&state.transactionID, &state.tokenStateHash, &state.latestPosition, &state.latestRole,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fullNodeTokenState{}, nil
			}
			return fullNodeTokenState{}, fmt.Errorf("fullnode persistence: read fullnode_nft token %q: %w", tokenID, err)
		}
		state.exists = true
		return state, nil
	case constants.TokenType_SmartContract:
		var state fullNodeTokenState
		err := tx.QueryRow(ctx, `
			SELECT token_id, token_value, token_status, transaction_id,
			       token_state_hash, latest_position, latest_role
			FROM fullnode_smart_contract
			WHERE token_id = $1
		`, tokenID).Scan(
			&state.tokenID, &state.tokenValue, &state.tokenStatus,
			&state.transactionID, &state.tokenStateHash, &state.latestPosition, &state.latestRole,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fullNodeTokenState{}, nil
			}
			return fullNodeTokenState{}, fmt.Errorf("fullnode persistence: read fullnode_smart_contract token %q: %w", tokenID, err)
		}
		state.exists = true
		return state, nil
	default:
		return fullNodeTokenState{}, fmt.Errorf("fullnode persistence: unsupported token type %q", tokenType)
	}
}

func (w *Wallet) upsertFullNodeTokenStateTx(ctx context.Context, tx pgx.Tx, tokenType string, state fullNodeTokenState) error {
	switch tokenType {
	case constants.TokenType_RBT:
		_, err := tx.Exec(ctx, `
			INSERT INTO fullnode_rbt (
				token_id, parent_token_id, token_value, token_status, did, transaction_id,
				token_state_hash, latest_position, latest_role, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW())
			ON CONFLICT (token_id) DO UPDATE SET
				parent_token_id = EXCLUDED.parent_token_id,
				token_value = EXCLUDED.token_value,
				token_status = EXCLUDED.token_status,
				did = EXCLUDED.did,
				transaction_id = EXCLUDED.transaction_id,
				token_state_hash = EXCLUDED.token_state_hash,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`, state.tokenID, state.parentTokenID, state.tokenValue, state.tokenStatus, state.did, state.transactionID, state.tokenStateHash, state.latestPosition, state.latestRole)
		if err != nil {
			return fmt.Errorf("fullnode persistence: upsert fullnode_rbt token %q: %w", state.tokenID, err)
		}
		return nil
	case constants.TokenType_FT:
		_, err := tx.Exec(ctx, `
			INSERT INTO fullnode_ft (
				token_id, token_value, token_status, did, transaction_id,
				token_state_hash, latest_position, latest_role, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW(),NOW())
			ON CONFLICT (token_id) DO UPDATE SET
				token_value = EXCLUDED.token_value,
				token_status = EXCLUDED.token_status,
				did = EXCLUDED.did,
				transaction_id = EXCLUDED.transaction_id,
				token_state_hash = EXCLUDED.token_state_hash,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`, state.tokenID, state.tokenValue, state.tokenStatus, state.did, state.transactionID, state.tokenStateHash, state.latestPosition, state.latestRole)
		if err != nil {
			return fmt.Errorf("fullnode persistence: upsert fullnode_ft token %q: %w", state.tokenID, err)
		}
		return nil
	case constants.TokenType_NFT:
		_, err := tx.Exec(ctx, `
			INSERT INTO fullnode_nft (
				token_id, token_value, token_status, did, transaction_id,
				token_state_hash, latest_position, latest_role, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW(),NOW())
			ON CONFLICT (token_id) DO UPDATE SET
				token_value = EXCLUDED.token_value,
				token_status = EXCLUDED.token_status,
				did = EXCLUDED.did,
				transaction_id = EXCLUDED.transaction_id,
				token_state_hash = EXCLUDED.token_state_hash,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`, state.tokenID, state.tokenValue, state.tokenStatus, state.did, state.transactionID, state.tokenStateHash, state.latestPosition, state.latestRole)
		if err != nil {
			return fmt.Errorf("fullnode persistence: upsert fullnode_nft token %q: %w", state.tokenID, err)
		}
		return nil
	case constants.TokenType_SmartContract:
		_, err := tx.Exec(ctx, `
			INSERT INTO fullnode_smart_contract (
				token_id, token_value, token_status, transaction_id,
				token_state_hash, latest_position, latest_role, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),NOW())
			ON CONFLICT (token_id) DO UPDATE SET
				token_value = EXCLUDED.token_value,
				token_status = EXCLUDED.token_status,
				transaction_id = EXCLUDED.transaction_id,
				token_state_hash = EXCLUDED.token_state_hash,
				latest_position = EXCLUDED.latest_position,
				latest_role = EXCLUDED.latest_role,
				updated_at = NOW()
		`, state.tokenID, state.tokenValue, state.tokenStatus, state.transactionID, state.tokenStateHash, state.latestPosition, state.latestRole)
		if err != nil {
			return fmt.Errorf("fullnode persistence: upsert fullnode_smart_contract token %q: %w", state.tokenID, err)
		}
		return nil
	default:
		return fmt.Errorf("fullnode persistence: unsupported token type %q", tokenType)
	}
}

func (w *Wallet) insertFullNodeTokenChainRows(ctx context.Context, tx pgx.Tx, rows []models.TokenChain) error {
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fullnode_tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (token_id, position) DO NOTHING
		`, row.TokenID, row.TransactionID, row.PreviousTransactionID, row.Role, row.Position); err != nil {
			return fmt.Errorf("fullnode persistence: insert fullnode_tokenchain row for token %q: %w", row.TokenID, err)
		}
	}
	return nil
}

func (w *Wallet) syncFullNodeTokenChainIndex(ctx context.Context, tx pgx.Tx, tokenIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT token_id, array_agg(id ORDER BY position)
		FROM fullnode_tokenchain
		WHERE token_id = ANY($1::text[])
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return fmt.Errorf("fullnode persistence: read fullnode tokenchain index: %w", err)
	}
	defer rows.Close()

	type indexRow struct {
		tokenID string
		index   []int32
	}
	indexRows := make([]indexRow, 0, len(tokenIDs))
	for rows.Next() {
		var row indexRow
		if err := rows.Scan(&row.tokenID, &row.index); err != nil {
			return fmt.Errorf("fullnode persistence: scan fullnode tokenchain index: %w", err)
		}
		indexRows = append(indexRows, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("fullnode persistence: stream fullnode tokenchain index: %w", err)
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
		INSERT INTO fullnode_tokenchain_index (token_id, index, created_at, updated_at)
		VALUES ` + strings.Join(placeholders, ",") + `
		ON CONFLICT (token_id) DO UPDATE SET
			index = EXCLUDED.index,
			updated_at = NOW()
	`
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("fullnode persistence: upsert fullnode tokenchain index: %w", err)
	}
	return nil
}

// func (w *Wallet) GetFullNodeTransactionByID(ctx context.Context, tx pgx.Tx, transactionID string) (*models.Transactions, error) {
// 	var transaction models.Transactions
// 	err := tx.QueryRow(ctx, `
// 		SELECT id, info, signature, created_at, updated_at
// 		FROM fullnode_transactions
// 		WHERE id = $1
// 	`, transactionID).Scan(&transaction.ID, &transaction.Info, &transaction.Signature, &transaction.CreatedAt, &transaction.UpdatedAt)
// 	if err != nil {
// 		return nil, fmt.Errorf("fullnode persistence: read fullnode transaction %q: %w", transactionID, err)
// 	}
// 	return &transaction, nil
// }

func (w *Wallet) GetFullNodeTokenChainByTokenID(tokenID string) ([]models.TokenChain, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT id, token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at
		 FROM fullnode_tokenchain WHERE token_id = $1 ORDER BY position ASC`, tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetFullNodeTokenChainByTokenID: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.TokenChain])
}

// PersistFullNodeSyncedTokenChain atomically writes synced transactions and
// tokenchain entries for a single token into fullnode tables, then updates
// the token state and tokenchain index. This is the fullnode counterpart of
// ApplyTokenChainBatch + CreateTransactionIfNotExists used in the normal path.
func (w *Wallet) PersistFullNodeSyncedTokenChain(
	ctx context.Context,
	tokenID string,
	newTxs []models.Transactions,
	entries []models.TokenChain,
	lastTxInfo *models.TransactionInfo,
	tokenType string,
	lastRole int16,
) error {
	if len(newTxs) == 0 {
		return nil
	}

	tx, err := w.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("fullnode sync persistence: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for i := range newTxs {
		if err := w.insertFullNodeTransaction(ctx, tx, &newTxs[i]); err != nil {
			return err
		}
	}

	if err := w.insertFullNodeTokenChainRows(ctx, tx, entries); err != nil {
		return err
	}

	if err := w.syncFullNodeTokenChainIndex(ctx, tx, []string{tokenID}); err != nil {
		return err
	}

	lastEntry := entries[len(entries)-1]

	// Locate the synced token in the transaction to decide the correct
	// owner DID and tokenValue to persist. Resolution order:
	//
	//   1. lastTxInfo.Tokens.{RBT,FT,NFT,SmartContract}
	//        → transfer/committed token — owner = lastTxInfo.Owner
	//
	//   2. lastTxInfo.Quorums[].Tokens
	//        → pledge token of the LAST synced transaction
	//        → owner = that QuorumInfo.Did
	//        → value from QuorumInfo entry if non-zero, else derive from tokenID
	//
	//If token is not present in lastTransactionInfo, which means for the given
	//transactionInfo it is the unpledged token, so it won't be there.
	//In that case, search the token in the previous txnInfo(which is pledged txn) and update the token details accordingly.
	//   3. entries' last PreviousTransactionID → find that tx in newTxs →
	//      inspect its Quorums[].Tokens
	//        → pledge token created by an earlier tx in this sync batch
	//        → owner = that QuorumInfo.Did
	//        → value from QuorumInfo entry if non-zero, else derive from tokenID
	var (
		tokenValue     float64
		tokenStateHash string
		pledgeOwnerDID string
		foundIn        string // "transfer" | "pledge:last" | "pledge:prev" | ""
	)

	// Step 1: transfer / committed token lookup in the last transaction.
	if lastTxInfo != nil && lastTxInfo.Tokens != nil {
		transferLists := [][]*models.TokenInfo{
			lastTxInfo.Tokens.RBT,
			lastTxInfo.Tokens.FT,
			lastTxInfo.Tokens.NFT,
			lastTxInfo.Tokens.SmartContract,
		}
		for _, list := range transferLists {
			for _, t := range list {
				if t != nil && t.TokenID == tokenID {
					tokenValue = t.TokenValue
					tokenStateHash = t.Data
					foundIn = "transfer"
					break
				}
			}
			if foundIn != "" {
				break
			}
		}
	}

	// Step 2: pledge token lookup in lastTxInfo.Quorums[].Tokens.
	if foundIn == "" && lastTxInfo != nil {
		for _, q := range lastTxInfo.Quorums {
			if q == nil {
				continue
			}
			for _, t := range q.Tokens {
				if t == nil || t.TokenID != tokenID {
					continue
				}
				if t.TokenValue > 0 {
					tokenValue = t.TokenValue
				} else {
					v, err := util.GetTokenValueFromTokenID(tokenID)
					if err != nil {
						return fmt.Errorf("fullnode sync persistence: derive token value for pledge token %q: %w", tokenID, err)
					}
					tokenValue = v
				}
				tokenStateHash = t.Data
				pledgeOwnerDID = q.Did
				foundIn = "pledge:last"
				break
			}
			if foundIn != "" {
				break
			}
		}
	}

	// Step 3: pledge token lookup in the previous transaction referenced by the
	// last chain entry. The previous tx must be part of this sync batch (newTxs).
	if foundIn == "" && lastEntry.PreviousTransactionID != nil && *lastEntry.PreviousTransactionID != "" {
		prevTxID := *lastEntry.PreviousTransactionID
		for i := range newTxs {
			if newTxs[i].ID != prevTxID {
				continue
			}
			var prevTxInfo models.TransactionInfo
			if err := json.Unmarshal(newTxs[i].Info, &prevTxInfo); err != nil {
				return fmt.Errorf("fullnode sync persistence: unmarshal prev tx %q info: %w", prevTxID, err)
			}
			for _, q := range prevTxInfo.Quorums {
				if q == nil {
					continue
				}
				for _, t := range q.Tokens {
					if t == nil || t.TokenID != tokenID {
						continue
					}
					if t.TokenValue > 0 {
						tokenValue = t.TokenValue
					} else {
						v, err := util.GetTokenValueFromTokenID(tokenID)
						if err != nil {
							return fmt.Errorf("fullnode sync persistence: derive token value for pledge token %q (via prev tx %q): %w", tokenID, prevTxID, err)
						}
						tokenValue = v
					}
					tokenStateHash = t.Data
					pledgeOwnerDID = q.Did
					foundIn = "pledge:prev"
					break
				}
				if foundIn != "" {
					break
				}
			}
			break
		}
	}

	tokenState, err := w.readFullNodeTokenStateTx(ctx, tx, tokenType, tokenID)
	if err != nil {
		return err
	}
	if !tokenState.exists {
		tokenState.tokenID = tokenID
		tokenState.tokenStatus = int16(constants.TokenStatus_Free)
	}
	if tokenValue > 0 {
		tokenState.tokenValue = tokenValue
	}
	if tokenStateHash != "" {
		tokenState.tokenStateHash = tokenStateHash
	}
	switch foundIn {
	case "pledge:last", "pledge:prev":
		if pledgeOwnerDID != "" {
			tokenState.did = pledgeOwnerDID
		}
	default:
		if lastTxInfo != nil && lastTxInfo.Owner != "" {
			tokenState.did = lastTxInfo.Owner
		}
	}
	tokenState.transactionID = lastEntry.TransactionID
	tokenState.latestPosition = lastEntry.Position
	tokenState.latestRole = lastRole
	tokenState.exists = true

	if err := w.upsertFullNodeTokenStateTx(ctx, tx, tokenType, tokenState); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("fullnode sync persistence: commit: %w", err)
	}
	return nil
}
