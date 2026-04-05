package wallet

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (w *Wallet) PledgeTokens(tokenInfos []*models.TokenInfo, transaction *models.Transactions, did string, epoch int64) error {
	// Sanity check
	if len(tokenInfos) == 0 {
		return fmt.Errorf("PledgeTokens(tx=%v): unexpected error: no tokens are being passed to pledge", transaction.ID)
	}
	tokenIDs := func() []string {
		var tokens []string = make([]string, 0)

		for _, tokenInfo := range tokenInfos {
			tokens = append(tokens, tokenInfo.TokenID)
		}

		return tokens
	}()

	// Checking if the input tokenInfos carries duplicate tokens
	// which is an unexpected behaviour
	seen := make(map[string]struct{})
	for _, tokenInfo := range tokenInfos {
		if _, exists := seen[tokenInfo.TokenID]; exists {
			return fmt.Errorf("PledgeTokens(tx=%v): unexpected error: duplicate token_id detected: %s", transaction.ID, tokenInfo.TokenID)
		}
		seen[tokenInfo.TokenID] = struct{}{}
	}

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return fmt.Errorf("PledgeTokens(tx=%v): failed to get the tx Object, err: %v", transaction.ID, err)
	}
	defer tx.Rollback(w.Ctx)

	// Phase 1: Check if the Pledge Tokens are in locked state.
	var lockedTokensCount int
	err = tx.QueryRow(w.Ctx, `
		SELECT COUNT(DISTINCT token_id)
		FROM tokens
		WHERE token_id=ANY($1)
		AND token_status=$2
	`, tokenIDs, constants.TokenStatus_Locked).Scan(&lockedTokensCount)
	if err != nil {
		return err
	}

	if len(tokenIDs) != lockedTokensCount {
		return fmt.Errorf("PledgeTokens(tx=%v): some of the tokens were not found to be in locked state. List of tokens: %v", transaction.ID, tokenIDs)
	}

	// Phase 2: Set the token status to Pledged
	if _, err := tx.Exec(w.Ctx, `
		UPDATE tokens
		SET token_status=$1
		WHERE token_id=ANY($2)
	`, constants.TokenStatus_Pledged, tokenIDs); err != nil {
		return fmt.Errorf("PledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", transaction.ID, err)
	}

	// Phase 3: Add the incoming transaction to the `transactions` table
	// ###--- commented for new unpledge logic
	// if _, err := tx.Exec(w.Ctx, `
	// 	INSERT INTO transactions (id, info, signature)
	// 	VALUES ($1, $2, $3)
	// `, transaction.ID, transaction.Info, transaction.Signature); err != nil {
	// 	return fmt.Errorf("PledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", transaction.ID, err)
	// }

	var origTxInfo models.TransactionInfo
	if err := json.Unmarshal(transaction.Info, &origTxInfo); err != nil {
		origTxInfo.Network = ""
	}

	pledgeTokenInfos := make([]*models.TokenInfo, 0, len(tokenInfos))
	for _, ti := range tokenInfos {
		tokenValue, _ := util.GetTokenValueFromTokenID(ti.TokenID)
		pledgeTokenInfos = append(pledgeTokenInfos, &models.TokenInfo{
			TokenID:               ti.TokenID,
			PreviousTransactionID: ti.PreviousTransactionID,
			TokenValue:            tokenValue,
			DID:                   did,
		})
	}

	pledgeTxInfo := &models.TransactionInfo{
		Initiator: did,
		Owner:     did,
		Epoch:     int(epoch),
		Network:   origTxInfo.Network,
		Tokens: &models.TransactionTokens{
			RBT: pledgeTokenInfos,
		},
		Memo: fmt.Sprintf("PLEDGE for_tx=%s", transaction.ID),
	}

	pledgeInfoBytes, err := models.SerializeTransactionInfo(pledgeTxInfo)
	if err != nil {
		return fmt.Errorf("PledgeTokens(tx=%v): failed to serialize pledge TransactionInfo: %v", transaction.ID, err)
	}
	transaction.Info = pledgeInfoBytes

	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO transactions (id, info, signature)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, transaction.ID, transaction.Info, transaction.Signature); err != nil {
		return fmt.Errorf("PledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", transaction.ID, err)
	}

	// Phase 3: Populate the transactionID with pledge type role in `tokenchain` table for all pledge tokens
	// and get the `id` column values for the next phase

	// Get the next position for tokens
	rowsNexPos, err := tx.Query(w.Ctx, `
		SELECT token_id, COALESCE(MAX(position), 0) + 1 AS next_position
		FROM tokenchain
		WHERE token_id = ANY($1)
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return err
	}
	defer rowsNexPos.Close()

	tokenNextPositionMap := make(map[string]int64)

	for rowsNexPos.Next() {
		var tokenID string
		var nextPos int64

		if err := rowsNexPos.Scan(&tokenID, &nextPos); err != nil {
			return err
		}

		tokenNextPositionMap[tokenID] = nextPos
	}

	// Update the tokenchain index and get the `id` column value in a map
	pledgeTypeNum := models.GetTokenRoleID(constants.TokenRole_Pledge)

	prevTxnIDs := make([]*string, 0, len(tokenInfos))
	roles := make([]int16, 0, len(tokenInfos))
	positions := make([]int, 0, len(tokenInfos))
	// ###--- commented for new prev_tx_from_tokenchain logic
	/*
		 for _, tokenInfo := range tokenInfos {
			prevTxnIDs = append(prevTxnIDs, &tokenInfo.PreviousTransactionID)
			roles = append(roles, int16(pledgeTypeNum))
			positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
		} */

	/* for _, tokenInfo := range tokenInfos {
			var prevTx string

			err := tx.QueryRow(w.Ctx, `
	        SELECT transaction_id
	        FROM tokenchain
	        WHERE token_id = $1
	        ORDER BY position DESC
	        LIMIT 1
	    `, tokenInfo.TokenID).Scan(&prevTx)
			if err != nil {
				return fmt.Errorf("PledgeTokens: failed to fetch latest tokenchain tx for token %s: %v", tokenInfo.TokenID, err)
			}

			prevTxnIDs = append(prevTxnIDs, &prevTx)
			roles = append(roles, int16(pledgeTypeNum))
			positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
		}
	*/
	for _, tokenInfo := range tokenInfos {
		var prevTx string

		err := tx.QueryRow(w.Ctx, `
        SELECT transaction_id
        FROM tokenchain
        WHERE token_id = $1
        ORDER BY position DESC
        LIMIT 1
    `, tokenInfo.TokenID).Scan(&prevTx)
		if err != nil {
			return fmt.Errorf("PledgeTokens: failed to fetch latest tokenchain tx for token %s: %v", tokenInfo.TokenID, err)
		}

		prevTxnIDs = append(prevTxnIDs, &prevTx)
		roles = append(roles, int16(pledgeTypeNum))
		positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
	}
	// Phase 4: Add the `id` column values to the respective pledge token's array in `tokenchain_index` column
	rows, err := tx.Query(
		w.Ctx,
		`
		INSERT INTO tokenchain (
			token_id, 
			transaction_id, 
			previous_transaction_id, 
			role, 
			position
		)
		SELECT 
			t.token_id,
			$1,
			t.prev_txn_id,
			t.role,
			t.position
		FROM 
			UNNEST(
				$2::TEXT[],
				$3::TEXT[],
				$4::SMALLINT[],
				$5::INT[]
			) AS t(token_id, prev_txn_id, role, position)
		RETURNING id, token_id;
		`,
		transaction.ID,
		tokenIDs,
		prevTxnIDs,
		roles,
		positions,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tokenchainRowIDMap map[string]int = make(map[string]int)
	var tokenDenomMap map[types.DenomValue]types.DenomCount = make(map[types.DenomValue]types.DenomCount)

	for rows.Next() {
		var id int
		var tokenId string

		if err := rows.Scan(&id, &tokenId); err != nil {
			return err
		}

		tokenValue, err := util.GetTokenValueFromTokenID(tokenId)
		if err != nil {
			return err
		}

		if _, exists := tokenDenomMap[tokenValue]; !exists {
			tokenDenomMap[tokenValue] = 1
		} else {
			tokenDenomMap[tokenValue] += 1
		}

		tokenchainRowIDMap[tokenId] = id
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tokenIDList, tokenIDRow := util.UnzipMap(tokenchainRowIDMap)
	_, err = tx.Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index)
		SELECT 
			t.token_id,
			ARRAY[t.idx]
		FROM
			UNNEST($1::TEXT[], $2::INTEGER[]) AS t(token_id, idx)
		ON CONFLICT (token_id)
		DO UPDATE
		SET 
			index = tokenchain_index.index || EXCLUDED.index,
			updated_at = NOW();
	`, tokenIDList, tokenIDRow)
	if err != nil {
		return err
	}

	// Phase 5: Update the Token Denom Array table
	denomValueList, denomCountList := util.UnzipMap(tokenDenomMap)
	_, err = tx.Exec(w.Ctx, `
		UPDATE token_denom t
		SET 
			count = t.count - d.count,
			updated_at = NOW()
		FROM 
			UNNEST($2::NUMERIC[], $3::BIGINT[]) AS d(denom, count)
		WHERE 
			t.did = $1
			AND t.denom = d.denom;
	`, did, denomValueList, denomCountList)
	if err != nil {
		return err
	}

	// Phase 6: Update the unpledge_sequence_info table
	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO unpledge_sequence_info(tx_id, pledge_tokens, epoch, quorum_did)
		VALUES ($1, $2, $3, $4)
	`, transaction.ID, tokenIDs, epoch, did); err != nil {
		return err
	}

	if err := tx.Commit(w.Ctx); err != nil {
		return fmt.Errorf("PledgeTokens(tx=%v): commit failed, err: %v", transaction.ID, err)
	}

	return nil
}

func (w *Wallet) UnpledgeTokens(prevTransactionId string, transaction *models.Transactions, did string) error {

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to get the tx Object, err: %v", prevTransactionId, err)
	}
	defer tx.Rollback(w.Ctx)

	// Idempotency guard: if the unpledge_sequence_info row is already gone,
	// this unpledge completed in a prior invocation — return success silently.
	var seqExists bool
	if err := tx.QueryRow(w.Ctx, `
		SELECT EXISTS(SELECT 1 FROM unpledge_sequence_info WHERE tx_id=$1)
	`, prevTransactionId).Scan(&seqExists); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to check unpledge_sequence_info, err: %v", prevTransactionId, err)
	}
	if !seqExists {
		w.log.Info("UnpledgeTokens: idempotent skip — unpledge already completed", "prevTxID", prevTransactionId)
		return nil
	}

	// Phase 0: Get tokens from unpledge_sequence_table
	var tokenIDs []string = make([]string, 0)
	var pledgeEpoch int
	if err := tx.QueryRow(w.Ctx, `
		SELECT pledge_tokens, epoch FROM unpledge_sequence_info
		WHERE tx_id=$1
	`, prevTransactionId).Scan(&tokenIDs, &pledgeEpoch); err != nil {
		return err
	}

	var tokenInfos []models.TokenInfo = make([]models.TokenInfo, 0)
	// ###--- commented for new prev_tx_from_tokenchain logic
	/*
		rowTokenInfo, err := tx.Query(w.Ctx, `
			SELECT token_id, transaction_id FROM tokens
			WHERE token_id=ANY($1)
		`, tokenIDs) */
	rowTokenInfo, err := tx.Query(w.Ctx, `
    SELECT tc.token_id, tc.transaction_id
    FROM tokenchain tc
    JOIN (
        SELECT token_id, MAX(position) as max_pos
        FROM tokenchain
        WHERE token_id = ANY($1)
        GROUP BY token_id
    ) latest
    ON tc.token_id = latest.token_id AND tc.position = latest.max_pos
`, tokenIDs)
	if err != nil {
		return err
	}
	defer rowTokenInfo.Close()

	for rowTokenInfo.Next() {
		var tokenId string
		var transactionId string
		if err := rowTokenInfo.Scan(&tokenId, &transactionId); err != nil {
			return err
		}

		tokenInfos = append(tokenInfos, models.TokenInfo{
			TokenID:               tokenId,
			PreviousTransactionID: transactionId,
		})
	}

	// Phase 1: Check if the tokens are in pledged state or not
	/* var areAllTokensPledged bool = false

	if err := tx.QueryRow(w.Ctx, `
		SELECT COUNT(DISTINCT token_id) =$2
		FROM tokens
		WHERE token_id=ANY($1)
		AND token_status=$2
	`, tokenIDs, constants.TokenStatus_Pledged).Scan(&areAllTokensPledged); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to scan for query to check if all tokens are pledged, err: %v", prevTransactionId, err)
	}
	if !areAllTokensPledged {
		return fmt.Errorf("UnpledgeTokens(tx=%v): not all tokens were found to be in pledged state, tokens: %v", prevTransactionId, tokenIDs)
	} */
	var count int64

	if err := tx.QueryRow(w.Ctx, `
    SELECT COUNT(DISTINCT token_id)
    FROM tokens
    WHERE token_id=ANY($1)
    AND token_status=$2
	`, tokenIDs, constants.TokenStatus_Pledged).Scan(&count); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to count pledged tokens, err: %v", prevTransactionId, err)
	}

	// ###--- commented for new unpledge logic
	// if count != int64(len(tokenIDs)) {
	// 	return fmt.Errorf("UnpledgeTokens(tx=%v): not all tokens are pledged, tokens: %v", prevTransactionId, tokenIDs)
	// }
	if count != int64(len(tokenIDs)) {
		// Check if tokens are already FREE — idempotent retry after a completed unpledge
		var freeCount int64
		if err := tx.QueryRow(w.Ctx, `
			SELECT COUNT(DISTINCT token_id)
			FROM tokens
			WHERE token_id=ANY($1)
			AND token_status=$2
		`, tokenIDs, constants.TokenStatus_Free).Scan(&freeCount); err != nil {
			return fmt.Errorf("UnpledgeTokens(tx=%v): failed to count free tokens, err: %v", prevTransactionId, err)
		}
		if freeCount == int64(len(tokenIDs)) {
			w.log.Info("UnpledgeTokens: idempotent skip — tokens already FREE", "prevTxID", prevTransactionId, "tokens", tokenIDs)
			return nil
		}
		return fmt.Errorf("UnpledgeTokens(tx=%v): not all tokens are pledged, tokens: %v", prevTransactionId, tokenIDs)
	}
	// Phase 2: Update Token status to 0
	if _, err := tx.Exec(w.Ctx, `
		UPDATE tokens
		SET token_status=$1
		WHERE token_id=ANY($2)
	`, constants.TokenStatus_Free, tokenIDs); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", prevTransactionId, err)
	}

	// Phase 3: Store the incoming transaction in `transactions` table
	// ###--- commented for new unpledge logic
	// if _, err := tx.Exec(w.Ctx, `
	// 	INSERT INTO transactions (id, info, signature)
	// 	VALUES ($1, $2, $3)
	// `, transaction.ID, transaction.Info, transaction.Signature); err != nil {
	// 	return fmt.Errorf("UnpledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", prevTransactionId, err)
	// }

	var origTxInfoUnpledge models.TransactionInfo
	if err := json.Unmarshal(transaction.Info, &origTxInfoUnpledge); err != nil {
		origTxInfoUnpledge.Network = ""
	}

	unpledgeTokenInfos := make([]*models.TokenInfo, 0, len(tokenInfos))
	for _, ti := range tokenInfos {
		tokenValue, _ := util.GetTokenValueFromTokenID(ti.TokenID)
		unpledgeTokenInfos = append(unpledgeTokenInfos, &models.TokenInfo{
			TokenID:               ti.TokenID,
			PreviousTransactionID: ti.PreviousTransactionID,
			TokenValue:            tokenValue,
			DID:                   did,
		})
	}

	unpledgeTxInfo := &models.TransactionInfo{
		Initiator: did,
		Owner:     did,
		Epoch:     pledgeEpoch,
		Network:   origTxInfoUnpledge.Network,
		Tokens: &models.TransactionTokens{
			RBT: unpledgeTokenInfos,
		},
		Memo: fmt.Sprintf("UNPLEDGE of_tx=%s", prevTransactionId),
	}

	unpledgeInfoBytes, err := models.SerializeTransactionInfo(unpledgeTxInfo)
	if err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to serialize unpledge TransactionInfo: %v", prevTransactionId, err)
	}
	transaction.Info = unpledgeInfoBytes

	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO transactions (id, info, signature)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, transaction.ID, transaction.Info, transaction.Signature); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to add transaction details in `transactions` table, err: %v", prevTransactionId, err)
	}

	// Phase 4: Update tokenchain table, with unpledge status

	// Get the next position for tokens
	rowsNexPos, err := tx.Query(w.Ctx, `
		SELECT token_id, COALESCE(MAX(position), 0) + 1 AS next_position
		FROM tokenchain
		WHERE token_id = ANY($1)
		GROUP BY token_id
	`, tokenIDs)
	if err != nil {
		return err
	}
	defer rowsNexPos.Close()

	tokenNextPositionMap := make(map[string]int64)

	for rowsNexPos.Next() {
		var tokenID string
		var nextPos int64

		if err := rowsNexPos.Scan(&tokenID, &nextPos); err != nil {
			return err
		}

		tokenNextPositionMap[tokenID] = nextPos
	}

	// Update the tokenchain index and get the `id` column value in a map
	unpledgeTypeNum := models.GetTokenRoleID(constants.TokenRole_Unpledge)

	prevTxnIDs := make([]*string, 0, len(tokenInfos))
	roles := make([]int16, 0, len(tokenInfos))
	positions := make([]int, 0, len(tokenInfos))
	// ###--- commented for new prev_tx_from_tokenchain logic
	/*
		for _, tokenInfo := range tokenInfos {
			prevTxnIDs = append(prevTxnIDs, &tokenInfo.PreviousTransactionID)
			roles = append(roles, int16(unpledgeTypeNum))
			positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
		} */

	/* for _, tokenInfo := range tokenInfos {
			var prevTx string

			err := tx.QueryRow(w.Ctx, `
	        SELECT transaction_id
	        FROM tokenchain
	        WHERE token_id = $1
	        ORDER BY position DESC
	        LIMIT 1
	    `, tokenInfo.TokenID).Scan(&prevTx)
			if err != nil {
				return fmt.Errorf("PledgeTokens: failed to fetch latest tokenchain tx for token %s: %v", tokenInfo.TokenID, err)
			}

			prevTxnIDs = append(prevTxnIDs, &prevTx)
			roles = append(roles, int16(unpledgeTypeNum))
			positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
		} */
	for _, tokenInfo := range tokenInfos {
		var prevTx string

		err := tx.QueryRow(w.Ctx, `
        SELECT transaction_id
        FROM tokenchain
        WHERE token_id = $1
        ORDER BY position DESC
        LIMIT 1
    `, tokenInfo.TokenID).Scan(&prevTx)
		if err != nil {
			return fmt.Errorf("UnpledgeTokens: failed to fetch latest tokenchain tx for token %s: %v", tokenInfo.TokenID, err)
		}

		prevTxnIDs = append(prevTxnIDs, &prevTx)
		roles = append(roles, int16(unpledgeTypeNum))
		positions = append(positions, int(tokenNextPositionMap[tokenInfo.TokenID]))
	}
	// Phase 4: Add the `id` column values to the respective pledge token's array in `tokenchain_index` column
	rows, err := tx.Query(
		w.Ctx,
		`
		INSERT INTO tokenchain (
			token_id,
			transaction_id,
			previous_transaction_id,
			role,
			position
		)
		SELECT
			t.token_id,
			$1,
			t.prev_txn_id,
			t.role,
			t.position
		FROM
			UNNEST(
				$2::TEXT[],
				$3::TEXT[],
				$4::SMALLINT[],
				$5::INT[]
			) AS t(token_id, prev_txn_id, role, position)
		RETURNING id, token_id;
		`,
		transaction.ID,
		tokenIDs,
		prevTxnIDs,
		roles,
		positions,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tokenchainRowIDMap map[string]int = make(map[string]int)
	var tokenDenomMap map[types.DenomValue]types.DenomCount = make(map[types.DenomValue]types.DenomCount)

	for rows.Next() {
		var id int
		var tokenId string

		if err := rows.Scan(&id, &tokenId); err != nil {
			return err
		}

		tokenValue, err := util.GetTokenValueFromTokenID(tokenId)
		if err != nil {
			return err
		}

		if _, exists := tokenDenomMap[tokenValue]; !exists {
			tokenDenomMap[tokenValue] = 1
		} else {
			tokenDenomMap[tokenValue] += 1
		}

		tokenchainRowIDMap[tokenId] = id
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Phase 5: Update tokenchain_index table
	tokenIDList, tokenIDRow := util.UnzipMap(tokenchainRowIDMap)
	_, err = tx.Exec(w.Ctx, `
		INSERT INTO tokenchain_index (token_id, index)
		SELECT
			t.token_id,
			ARRAY[t.idx]
		FROM
			UNNEST($1::TEXT[], $2::INTEGER[]) AS t(token_id, idx)
		ON CONFLICT (token_id)
		DO UPDATE
		SET
			index = tokenchain_index.index || EXCLUDED.index,
			updated_at = NOW();
	`, tokenIDList, tokenIDRow)
	if err != nil {
		return err
	}

	w.log.Info("UNPLEDGE_TRIGGER",
		"prev_tx", prevTransactionId,
		"tokens", tokenIDs,
	)
	w.log.Info("DENOM_UPDATE",
		"did", did,
		"denoms", tokenDenomMap,
	)
	// uncommenting now
	// // Phase 6: Update token_denom table
	denomValueList, denomCountList := util.UnzipMap(tokenDenomMap)
	_, err = tx.Exec(w.Ctx, `
		INSERT INTO token_denom (did, denom, count)
		SELECT
			$1,
			d.denom,
			d.count
		FROM
			UNNEST($2::NUMERIC[], $3::BIGINT[]) AS d(denom, count)
		ON CONFLICT (did, denom)
		DO UPDATE
		SET
			count = token_denom.count + EXCLUDED.count,
			updated_at = NOW();
	`, did, denomValueList, denomCountList)
	if err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): unable to update token_denom, err: %v", prevTransactionId, err)
	}

	// Phase 7: Delete the unpledge_sequence_info record for the earlier transaction_id
	if _, err := tx.Exec(w.Ctx, `
		DELETE FROM unpledge_sequence_info
		WHERE tx_id = $1
	`, prevTransactionId); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): unable to remove record from unpledge_sequence_info, err: %v", prevTransactionId, err)
	}

	if err := tx.Commit(w.Ctx); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): commit failed, err: %v", prevTransactionId, err)
	}

	return nil
}

/* func (w *Wallet) UnpledgeTokens(prevTransactionId string, transaction *models.Transactions, did string) error {

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(w.Ctx)

	// Optional: log trigger (for debugging)
	w.log.Info("UNPLEDGE_TRIGGER",
		"prev_tx", prevTransactionId,
	)

	// Only cleanup
	if _, err := tx.Exec(w.Ctx, `
		DELETE FROM unpledge_sequence_info
		WHERE tx_id = $1
	`, prevTransactionId); err != nil {
		return err
	}

	return tx.Commit(w.Ctx)
} */

// CheckTxnsPresentInUnpledgeSequenceInfo checks if the provided transactions are
// present in the `unpledge_sequence_info` table or not. If they are, they are returned back
func (w *Wallet) CheckTxnsPresentInUnpledgeSequenceInfo(txs []string) ([]string, error) {
	rows, err := w.db.Pool().Query(
		w.Ctx,
		`
        SELECT tx_id
        FROM unpledge_sequence_info
        WHERE tx_id = ANY($1::TEXT[])
        `,
		txs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var txID string
		if err := rows.Scan(&txID); err != nil {
			return nil, err
		}
		result = append(result, txID)
	}

	return result, rows.Err()
}
