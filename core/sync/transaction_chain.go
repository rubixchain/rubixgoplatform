package sync

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/api"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// FindTokenRoleInTxn determines the role a token played in a given transaction.
func FindTokenRoleInTxn(tokenID string, txInfo *models.TransactionInfo) int16 {
	if txInfo.Tokens != nil {
		for _, lists := range [][]*models.TokenInfo{
			txInfo.Tokens.RBT, txInfo.Tokens.NFT,
			txInfo.Tokens.FT, txInfo.Tokens.SmartContract,
		} {
			for _, t := range lists {
				if t.TokenID == tokenID {
					return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
				}
			}
		}
	}

	for _, t := range txInfo.CommittedTokens {
		if t.TokenID == tokenID {
			return int16(models.GetTokenRoleID(constants.TokenRole_Commit))
		}
	}

	for _, q := range txInfo.Quorums {
		for _, t := range q.Tokens {
			if t.TokenID == tokenID {
				return int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
			}
		}
	}

	return int16(models.GetTokenRoleID(constants.TokenRole_Transfer))
}

func UpdateTokenRecord(w *wallet.Wallet, tokenID string, tokenType int, tokenValue float64, newOwnerDID string, latestTxID string, latestTxRole int, latestTxPosition int, log logger.Logger) error {
	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		log.Error("failed to begin transaction", "err", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(w.Ctx)

	var tokenExists bool
	_ = tx.QueryRow(w.Ctx,
		`SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=$1)`, tokenID,
	).Scan(&tokenExists)

	if tokenExists {
		if _, err := tx.Exec(w.Ctx,
			`UPDATE tokens
			SET did = $1, transaction_id = $2, latest_position = $3, latest_role = $4, updated_at = NOW()
			WHERE token_id = $5`,
			newOwnerDID, latestTxID, latestTxPosition, latestTxRole, tokenID,
		); err != nil {
			log.Error("failed to update token record", "err", err)
			return fmt.Errorf("failed to update token record: %w", err)
		}
	} else {
		// Expect only RBT tokens to have parent tokens
		var parenTokenIDObj pgtype.Text
		switch tokenType {
		case models.GetTokenTypeID(constants.TokenType_RBT):
			parentTokenID, err := parts.NewTokenIDFromString(tokenID).GetParentToken()
			if err != nil {
				log.Error("failed to get parent token ID", "err", err)
				return fmt.Errorf("failed to get parent token ID: %w", err)
			}
			parenTokenIDObj = pgtype.Text{String: parentTokenID, Valid: true}
		default:
			parenTokenIDObj = pgtype.Text{}
		}

		tokenRecord := models.Token{
			TokenID:        tokenID,
			ParentTokenID:  parenTokenIDObj,
			TokenValue:     tokenValue,
			TokenStatus:    constants.TokenStatus_Free,
			DID:            newOwnerDID,
			TransactionID:  latestTxID,
			TokenType:      int16(tokenType),
			LatestPosition: int64(latestTxPosition),
			LatestRole:     int16(latestTxRole),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Remove the following, only for DEBUG
		var txExists bool
		_ = tx.QueryRow(w.Ctx,
			`SELECT EXISTS(SELECT 1 FROM transactions WHERE id=$1)`, latestTxID,
		).Scan(&txExists)

		if !txExists {
			rows, err := tx.Query(w.Ctx, `SELECT * FROM transactions`)
			if err != nil {
				log.Error("failed to query transactions", "err", err)
				return fmt.Errorf("failed to query transactions: %w", err)
			}
			defer rows.Close()

			txes, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Transactions])
			if err != nil {
				log.Error("failed to collect transactions", "err", err)
				return fmt.Errorf("failed to collect transactions: %w", err)
			}

			log.Warn(fmt.Sprintf("All transactions in DB: %+v", txes))

			log.Error("Transaction does not exist in DB", "txID", latestTxID)
			return fmt.Errorf("transaction does not exist in DB: %s", latestTxID)
		} else {
			log.Warn("Transaction exists in DB", "txID", latestTxID)
		}

		if _, err := tx.Exec(w.Ctx,
			`INSERT INTO tokens(token_id, parent_token_id, token_value, token_status, did, transaction_id,
			token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			tokenRecord.TokenID, tokenRecord.ParentTokenID, tokenRecord.TokenValue, tokenRecord.TokenStatus,
			tokenRecord.DID, tokenRecord.TransactionID, tokenRecord.TokenStateHash, tokenRecord.TokenType,
			tokenRecord.LatestPosition, tokenRecord.LatestRole, tokenRecord.CreatedAt, tokenRecord.UpdatedAt,
		); err != nil {
			return fmt.Errorf("updateTokenRecord: failed to insert token record: %w", err)
		}
	}

	if _, err := tx.Exec(w.Ctx,
		`UPDATE tokens
		SET did = $1, transaction_id = $2, latest_position = $3, latest_role = $4, updated_at = NOW()
		WHERE token_id = $5`,
		newOwnerDID, latestTxID, latestTxPosition, latestTxRole, tokenID,
	); err != nil {
		log.Error("failed to update token record", "err", err)
		return fmt.Errorf("failed to update token record: %w", err)
	}

	if err = tx.Commit(w.Ctx); err != nil {
		log.Error("updateTokenRecord: failed to commit transaction", "err", err)
		return fmt.Errorf("updateTokenRecord: failed to commit transaction: %w", err)
	}

	return nil
}

type LatestTransactionInfoPostSync struct {
	Owner         string
	TransactionID string
	Role          int16
	Position      int64
}

// SyncTransactionChainFrom fetches missing transactions from a peer and writes them locally.
func SyncTransactionChainFrom(p *ipfsport.Peer, tokenID string, tokenType int, tokenValue float64, w *wallet.Wallet, log logger.Logger) (*LatestTransactionInfoPostSync, error) {
	latestTransactionInfoPostSync := &LatestTransactionInfoPostSync{}

	syncReq := models.TransactionChainSyncRequest{
		TokenID:       tokenID,
		TransactionID: "",
	}

	var tokenRecordExists bool = w.IsTokenExist(tokenID)

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		log.Error("failed to begin transaction", "err", err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	for {
		
		defer tx.Rollback(w.Ctx)
		// Call the Sync API to sync the transaction details of a token
		// from the peer
		var trep models.TransactionChainSyncResponse
		err = p.SendJSONRequest("POST", api.APISyncTransactionChain, nil, &syncReq, &trep, false)
		if err != nil {
			log.Error(fmt.Sprintf("APISyncTransactionChain: failed to sync transaction chain: %v", err))
			return nil, fmt.Errorf("APISyncTransactionChain: failed to sync transaction chain: %v", err)
		}
		if !trep.Status {
			log.Error(fmt.Sprintf("APISyncTransactionChain: failed to sync transaction chain: %s", trep.Message))
			return nil, fmt.Errorf("APISyncTransactionChain: failed to sync transaction chain: %s", trep.Message)
		}

		if len(trep.SyncTransactionInfoBytes) > 0 {
			var syncInfoList []models.SyncTransactionInfo
			for _, syncInfoBytes := range trep.SyncTransactionInfoBytes {
				var syncInfo models.SyncTransactionInfo

				if err = json.Unmarshal(syncInfoBytes, &syncInfo); err != nil {
					log.Error("failed to unmarshal sync transaction info bytes", "err", err)
					return nil, fmt.Errorf("failed to unmarshal sync transaction info bytes: %w", err)
				}

				syncInfoList = append(syncInfoList, syncInfo)
			}

			// Sort the list by position in ascending order to ensure correct sequence of writing
			// into the DB. We order based on the transaction position
			slices.SortFunc(syncInfoList, func(a, b models.SyncTransactionInfo) int {
				return cmp.Compare(a.Position, b.Position)
			})

			// Initially add all the transactions in `transactions`
			var rowIDList []int32 = make([]int32, 0)
			for _, syncInfo := range syncInfoList {
				var txInfo models.TransactionInfo
				if err = json.Unmarshal(syncInfo.Transaction.Info, &txInfo); err != nil {
					log.Error("failed to unmarshal transaction info", "err", err)
					return nil, fmt.Errorf("failed to unmarshal transaction info: %w", err)
				}

				// Update `transactions` table
				rowsAffected, err := tx.Exec(w.Ctx,
					`INSERT INTO transactions (id, info, signature, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
					syncInfo.Transaction.ID, syncInfo.Transaction.Info, syncInfo.Transaction.Signature, time.Now(), time.Now(),
				)
				if err != nil {
					log.Error("failed to insert transaction", "err", err)
					return nil, fmt.Errorf("failed to insert transaction: %w", err)
				}
				if rowsAffected.RowsAffected() == 0 {
					continue
				}

				latestTransactionInfoPostSync.TransactionID = syncInfo.Transaction.ID
			}

			// Add an initial token record if they dont exist.
			// Rest of the details will be updated when we add the token chain entries
			if !tokenRecordExists {
				var parenTokenIDObj pgtype.Text
				var tknValue float64 = tokenValue

				switch tokenType {
					case models.GetTokenTypeID(constants.TokenType_RBT):
						parentTokenID, err := parts.NewTokenIDFromString(tokenID).GetParentToken()
						if err != nil {
							log.Error("failed to get parent token ID", "err", err)
							return nil, fmt.Errorf("failed to get parent token ID: %w", err)
						}
						parenTokenIDObj = pgtype.Text{String: parentTokenID, Valid: true}

						tknValue, err = util.GetTokenValueFromTokenID(tokenID)
						if err != nil {
							log.Error("failed to get token value", "err", err)
							return nil, fmt.Errorf("failed to get token value: %w", err)
						}
					default:
						parenTokenIDObj = pgtype.Text{}
				}

				tokenRecord := models.Token{
					TokenID:        tokenID,
					ParentTokenID:  parenTokenIDObj,
					TokenValue:     tknValue,
					TokenStatus:    constants.TokenStatus_Free,
					DID:            p.GetPeerDID(),
					TransactionID:  latestTransactionInfoPostSync.TransactionID,
					TokenType:      int16(tokenType),
					LatestPosition: 0,
					LatestRole:     0,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}

				if _, err := tx.Exec(w.Ctx,
					`INSERT INTO tokens(token_id, parent_token_id, token_value, token_status, did, transaction_id,
					token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
					tokenRecord.TokenID, tokenRecord.ParentTokenID, tokenRecord.TokenValue, tokenRecord.TokenStatus,
					tokenRecord.DID, tokenRecord.TransactionID, tokenRecord.TokenStateHash, tokenRecord.TokenType,
					tokenRecord.LatestPosition, tokenRecord.LatestRole, tokenRecord.CreatedAt, tokenRecord.UpdatedAt,
				); err != nil {
					return nil, fmt.Errorf("updateTokenRecord: failed to insert token record: %w", err)
				}
			}

			// Then add entries into `tokenchain` table and update the token record with the latest transaction details
			for _, syncInfo := range syncInfoList {
				entry := &models.TokenChain{
					TokenID:               tokenID,
					TransactionID:         syncInfo.Transaction.ID,
					Role:                  syncInfo.Role,
					Position:              syncInfo.Position,
					PreviousTransactionID: syncInfo.PreviousTransactionID,
				}

				var rowID int32
				if err := tx.QueryRow(w.Ctx, `
					INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
					RETURNING id
				`, entry.TokenID, entry.TransactionID, entry.PreviousTransactionID, entry.Role, entry.Position).Scan(&rowID); err != nil {
					return nil, fmt.Errorf("AddTokenChainEntry: %w", err)
				}

				rowIDList = append(rowIDList, rowID)

				latestTransactionInfoPostSync.TransactionID = syncInfo.Transaction.ID
				latestTransactionInfoPostSync.Role = syncInfo.Role
				latestTransactionInfoPostSync.Position = syncInfo.Position
				latestTransactionInfoPostSync.Owner = p.GetPeerDID()
			}

			if len(rowIDList) > 0 {
				if _, err := tx.Exec(w.Ctx, `
					INSERT INTO tokenchain_index (token_id, index)
					VALUES ($1, $2)
					ON CONFLICT (token_id)
					DO UPDATE
					SET index = tokenchain_index.index || EXCLUDED.index,
						updated_at = NOW();
				`, tokenID, rowIDList); err != nil {
					return nil, fmt.Errorf("failed to update tokenchain index: %w", err)
				}
			}
		}

		if trep.NextTransactionID == "" {
			break
		}
		syncReq.TransactionID = trep.NextTransactionID
	}

	if _, err := tx.Exec(w.Ctx,
		`
			UPDATE tokens
			SET transaction_id = $1, latest_position = $2, latest_role = $3, updated_at = NOW()
			WHERE token_id = $4
		`,
		latestTransactionInfoPostSync.TransactionID, 
		latestTransactionInfoPostSync.Position, latestTransactionInfoPostSync.Role, tokenID,
	); err != nil {
		log.Error("failed to update token record", "err", err)
		return nil, fmt.Errorf("failed to update token record: %w", err)
	}

	if err := tx.Commit(w.Ctx); err != nil {
		log.Error("failed to commit transaction", "err", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return latestTransactionInfoPostSync, nil
}

func sortTokenIDs(tokenIDs []string) []string {
	levelCache := make(map[string]int)

	getLevel := func(t string) int {
		if lvl, ok := levelCache[t]; ok {
			return lvl
		}

		level := 0
		curr := t

		for {
			parent, _ := parts.NewTokenIDFromString(curr).GetParentToken()
			if parent == "" {
				break
			}
			level++
			curr = parent
		}

		levelCache[t] = level
		return level
	}

	// Sort by level (root first)
	sort.Slice(tokenIDs, func(i, j int) bool {
		return getLevel(tokenIDs[i]) < getLevel(tokenIDs[j])
	})

	return tokenIDs
}

func getCommittedTokensFromGenesisTx(genTx *models.TransactionInfo, tokenType int) ([]string, error) {
	var committedTokensIDs []string = make([]string, 0)

	if models.IsRBTToken(tokenType) {
		for _, t := range genTx.Tokens.RBT {
			committedTokensIDs = append(committedTokensIDs, t.TokenID)
		}

		return committedTokensIDs, nil
	}

	if models.IsFTToken(tokenType) {
		for _, t := range genTx.Tokens.FT {
			committedTokensIDs = append(committedTokensIDs, t.TokenID)
		}
		return committedTokensIDs, nil
	}

	return committedTokensIDs, nil
}

func getTokenRecordsToSync(ctx context.Context, tokenIDs []string, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT input.token_id
		FROM UNNEST($1::text[]) AS input(token_id)
		LEFT JOIN tokens t ON t.token_id = input.token_id
		WHERE t.token_id IS NULL;
	`, tokenIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missingTokens []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		missingTokens = append(missingTokens, id)
	}

	return missingTokens, nil
}
