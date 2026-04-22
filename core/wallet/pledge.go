package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (w *Wallet) PledgeTokens(tokenInfos []*models.TokenInfo, transaction *models.Transactions, did string, epoch int64) error {
	w.log.Info("PledgeTokens: Starting", "transactionID", transaction.ID, "did", did, "tokenCount", len(tokenInfos), "epoch", epoch)

	// Sanity check
	if len(tokenInfos) == 0 {
		w.log.Error("PledgeTokens: No tokens provided", "transactionID", transaction.ID)
		return fmt.Errorf("PledgeTokens(tx=%v): unexpected error: no tokens are being passed to pledge", transaction.ID)
	}
	tokenIDs := func() []string {
		var tokens []string = make([]string, 0)

		for _, tokenInfo := range tokenInfos {
			tokens = append(tokens, tokenInfo.TokenID)
		}

		return tokens
	}()

	w.log.Debug("PledgeTokens: Token IDs extracted", "transactionID", transaction.ID, "tokenIDs", tokenIDs)

	// Checking if the input tokenInfos carries duplicate tokens
	// which is an unexpected behaviour
	seen := make(map[string]struct{})
	for _, tokenInfo := range tokenInfos {
		if _, exists := seen[tokenInfo.TokenID]; exists {
			w.log.Error("PledgeTokens: Duplicate token detected", "transactionID", transaction.ID, "tokenID", tokenInfo.TokenID)
			return fmt.Errorf("PledgeTokens(tx=%v): unexpected error: duplicate token_id detected: %s", transaction.ID, tokenInfo.TokenID)
		}
		seen[tokenInfo.TokenID] = struct{}{}
	}

	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		w.log.Error("PledgeTokens: Failed to begin transaction", "transactionID", transaction.ID, "err", err)
		return fmt.Errorf("PledgeTokens(tx=%v): failed to get the tx Object, err: %v", transaction.ID, err)
	}
	defer tx.Rollback(w.Ctx)

	w.log.Debug("PledgeTokens: Phase 1 - Checking token lock status", "transactionID", transaction.ID)
	// Phase 1: Check if the Pledge Tokens are in locked state.
	var lockedTokensCount int
	err = tx.QueryRow(w.Ctx, `
		SELECT COUNT(DISTINCT token_id)
		FROM tokens
		WHERE token_id=ANY($1)
		AND token_status=$2
	`, tokenIDs, constants.TokenStatus_Locked).Scan(&lockedTokensCount)
	if err != nil {
		w.log.Error("PledgeTokens: Failed to query locked tokens", "transactionID", transaction.ID, "err", err)
		return err
	}

	w.log.Info("PledgeTokens: Lock status check", "transactionID", transaction.ID, "expectedCount", len(tokenIDs), "lockedCount", lockedTokensCount)

	if len(tokenIDs) != lockedTokensCount {
		w.log.Error("PledgeTokens: Not all tokens are in LOCKED state",
			"transactionID", transaction.ID,
			"expectedCount", len(tokenIDs),
			"lockedCount", lockedTokensCount,
			"tokenIDs", tokenIDs)
		return fmt.Errorf("PledgeTokens(tx=%v): some of the tokens were not found to be in locked state. List of tokens: %v", transaction.ID, tokenIDs)
	}

	w.log.Debug("PledgeTokens: Phase 2 - Updating token status to PLEDGED", "transactionID", transaction.ID)
	// Phase 2: Set the token status to Pledged
	if _, err := tx.Exec(w.Ctx, `
		UPDATE tokens
		SET token_status=$1
		WHERE token_id=ANY($2)
	`, constants.TokenStatus_Pledged, tokenIDs); err != nil {
		w.log.Error("PledgeTokens: Failed to update token status to PLEDGED", "transactionID", transaction.ID, "err", err)
		return fmt.Errorf("PledgeTokens(tx=%v): failed to update token status to PLEDGED, err: %v", transaction.ID, err)
	}
	w.log.Info("PledgeTokens: Tokens updated to PLEDGED status", "transactionID", transaction.ID, "tokenCount", len(tokenIDs))

	// Phase 3: Add the incoming transaction to the `transactions` table
	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO transactions (id, info, signature)
		VALUES ($1, $2, $3)
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

	for _, tokenInfo := range tokenInfos {
		prevTxnIDs = append(prevTxnIDs, &tokenInfo.PreviousTransactionID)
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
	w.log.Debug("PledgeTokens: Phase 6 - Inserting unpledge sequence info", "transactionID", transaction.ID)
	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO unpledge_sequence_info(tx_id, pledge_tokens, epoch, quorum_did)
		VALUES ($1, $2, $3, $4)
	`, transaction.ID, tokenIDs, epoch, did); err != nil {
		w.log.Error("PledgeTokens: Failed to insert unpledge sequence info", "transactionID", transaction.ID, "err", err)
		return err
	}
	w.log.Info("PledgeTokens: Unpledge sequence info inserted", "transactionID", transaction.ID, "quorumDID", did, "tokenIDs", tokenIDs)

	w.log.Debug("PledgeTokens: Committing transaction", "transactionID", transaction.ID)
	if err := tx.Commit(w.Ctx); err != nil {
		w.log.Error("PledgeTokens: Commit failed", "transactionID", transaction.ID, "err", err)
		return fmt.Errorf("PledgeTokens(tx=%v): commit failed, err: %v", transaction.ID, err)
	}

	w.log.Info("PledgeTokens: Successfully completed", "transactionID", transaction.ID, "tokenCount", len(tokenIDs))
	return nil
}

func (w *Wallet) UnpledgeTokens(prevTransactionId string, transaction *models.Transactions, did string) error {
	tx, err := w.BeginTx(w.Ctx)
	if err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to get the tx Object, err: %v", prevTransactionId, err)
	}
	defer tx.Rollback(w.Ctx)

	// Phase 0: Get tokens from unpledge_sequence_table
	var tokenIDs []string = make([]string, 0)
	if err := tx.QueryRow(w.Ctx, `
		SELECT pledge_tokens FROM unpledge_sequence_info
		WHERE tx_id=$1
	`, prevTransactionId).Scan(&tokenIDs); err != nil {
		return err
	}

	var tokenInfos []models.TokenInfo = make([]models.TokenInfo, 0)
	rowTokenInfo, err := tx.Query(w.Ctx, `
		SELECT token_id, transaction_id FROM tokens
		WHERE token_id=ANY($1)
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
	var areAllTokensPledged bool = false

	if err := tx.QueryRow(w.Ctx, `
		SELECT COUNT(DISTINCT token_id)
		FROM tokens
		WHERE token_id=ANY($1)
		AND token_status=$2
	`, tokenIDs, constants.TokenStatus_Pledged).Scan(&areAllTokensPledged); err != nil {
		return fmt.Errorf("UnpledgeTokens(tx=%v): failed to scan for query to check if all tokens are pledged, err: %v", prevTransactionId, err)
	}
	if !areAllTokensPledged {
		return fmt.Errorf("UnpledgeTokens(tx=%v): not all tokens were found to be in pledged state, tokens: %v", prevTransactionId, tokenIDs)
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
	if _, err := tx.Exec(w.Ctx, `
		INSERT INTO transactions (id, info, signature)
		VALUES ($1, $2, $3)
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

	for _, tokenInfo := range tokenInfos {
		prevTxnIDs = append(prevTxnIDs, &tokenInfo.PreviousTransactionID)
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

	// Phase 6: Update token_denom table
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

// CheckTxnsPresentInUnpledgeSequenceInfo checks if the provided transactions
// are present in the `unpledge_sequence_info` table AND are owned by the
// given quorum DID (outer ownership gate — defense-in-depth with UnpledgeV2's
// inner gate). Returns only tx_ids whose quorum_did column matches.
func (w *Wallet) CheckTxnsPresentInUnpledgeSequenceInfo(txs []string, quorumDID string, transactionTokensFromIncomingTx []string) ([]string, error) {
	rows, err := w.db.Pool().Query(
		w.Ctx,
		`
        SELECT tx_id, transaction_tokens
        FROM unpledge_sequence_info
        WHERE tx_id = ANY($1::TEXT[])
          AND quorum_did = $2
        `,
		txs, quorumDID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var txID string
		var storedTransactionTokens []string
		if err := rows.Scan(&txID, &storedTransactionTokens); err != nil {
			return nil, err
		}


		// Check if the unpledging is being done for the correct transaction tokens
		commonTokens := util.FindCommonElementsInList(transactionTokensFromIncomingTx, storedTransactionTokens)
		if len(commonTokens) == 0 {
			w.log.Warn("CheckTxnsPresentInUnpledgeSequenceInfo: transaction tokens from incoming unpledge transaction do not match with transaction tokens in unpledge_sequence_info — skip (not the correct transaction to unpledge for)",
				"txID", txID)
		} else {
			result = append(result, txID)
		}
	}

	return result, rows.Err()
}
