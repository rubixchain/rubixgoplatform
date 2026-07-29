package wallet

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

type PersistGenesisTransactionReq struct {
	DID                  string
	GenesisTokens        []GenesisMintRecord
	BurnTokens           []string
	GenesisTransaction   *models.Transactions
	MintTokensBeingBurnt []string
}

// PersistGenesisTransaction handles the database persistence of the genesis transaction 
// and its associated token state changes (burns and mints).
func (w *Wallet) PersistGenesisTransaction(req *PersistGenesisTransactionReq) error {
	tx, err := w.db.BeginTx(w.Ctx)
	if err != nil {
		return fmt.Errorf("PersistGenesisTransaction: failed to begin transaction, err: %v", err)
	}
	defer tx.Rollback(w.Ctx) //nolint:errcheck

	var denomArrayDelta map[float64]int = make(map[float64]int)

	for _, genesisRecord := range req.GenesisTokens {
		denomArrayDelta[genesisRecord.Token.TokenValue] += 1
	}
	for _, mintThenBurntToken := range req.MintTokensBeingBurnt {
		tokenValue, err := util.GetTokenValueFromTokenID(mintThenBurntToken)
		if err != nil {
			return fmt.Errorf("PersistGenesisTransaction: failed to get token value from token ID %s (mintThenBurntToken), err: %v", mintThenBurntToken, err)
		}

		denomArrayDelta[tokenValue] += 1
	}

	for _, burnTokenID := range req.BurnTokens {
		tokenValue, err := util.GetTokenValueFromTokenID(burnTokenID)
		if err != nil {
			return fmt.Errorf("PersistGenesisTransaction: failed to get token value from token ID %s, err: %v", burnTokenID, err)
		}
		denomArrayDelta[tokenValue] -= 1
	}

	// Sort the burn token list
	slices.SortFunc(req.BurnTokens, func(AtokenID, BtokenID string) int {
		aTokenValue, err := util.GetTokenValueFromTokenID(AtokenID)
		if err != nil {
			return 1
		}

		bTokenValue, err := util.GetTokenValueFromTokenID(BtokenID)
		if err != nil {
			return 1
		}

		return cmp.Compare(bTokenValue, aTokenValue)
	})

	// Update `transactions` table with the genesis transaction record
	if _, err := tx.Exec(w.Ctx,
		`INSERT INTO transactions (id, info, signature) VALUES ($1, $2, $3)`,
		req.GenesisTransaction.ID, req.GenesisTransaction.Info, req.GenesisTransaction.Signature,
	); err != nil {
		return fmt.Errorf("PersistGenesisTransaction: failed to insert genesis transaction, err: %v", err)
	}

	for _, burntToken := range req.BurnTokens {
		var parentTokenID string = ""
		tokenIDValue, _ := util.GetTokenValueFromTokenID(burntToken)
		if tokenIDValue != rubixmath.OneFloat() {
			parentTokenID, err = util.TokenID(burntToken).GetParentToken()
			if err != nil {
				return fmt.Errorf("PersistGenesisTransaction: failed to get parent token for token %v, err: %v", burntToken, err)
			}
		}

		if _, err := tx.Exec(w.Ctx,
			`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, transaction_id, token_state_hash, token_type, latest_role, latest_position, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (token_id) DO UPDATE SET token_status = EXCLUDED.token_status, latest_position = tokens.latest_position + 1, latest_role = EXCLUDED.latest_role, updated_at = EXCLUDED.updated_at
		`, burntToken, parentTokenID, tokenIDValue, constants.TokenStatus_Burnt,
			req.DID, req.GenesisTransaction.ID, "", models.GetTokenTypeID(constants.TokenType_RBT), models.GetTokenRoleID(constants.TokenRole_Burn), 0, time.Now(), time.Now()); err != nil {
			return fmt.Errorf("PersistGenesisTransaction: failed to insert burnt token record in 'tokens' table, err: %v", err)
		}
	}

	// Insert/Update the burnt tokens
	for _, tokenID := range req.BurnTokens {
		var returnedTokenID string
		var returnedID int

		// Update `tokenchain` table
		row := tx.QueryRow(w.Ctx,
			` 
			INSERT INTO tokenchain (
				token_id,
				transaction_id,
				previous_transaction_id,
				role,
				position
			)
			VALUES (
				$1,
				$2,
				(
					SELECT transaction_id
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				),
				$3,
				COALESCE((
					SELECT position
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				), -1) + 1
			) RETURNING token_id, id`,
			tokenID, req.GenesisTransaction.ID, models.GetTokenRoleID(constants.TokenRole_Burn),
		)
		if err := row.Scan(&returnedTokenID, &returnedID); err != nil {
			return fmt.Errorf("PersistPreConsensus: failed to insert token chain entry for burnt token ID %s, err: %v", tokenID, err)
		}

		// Update `tokenchain_index` table to add the new position for the burnt token
		if _, err := tx.Exec(w.Ctx, `
			INSERT INTO tokenchain_index (token_id, index)
			VALUES ($1, ARRAY[$2]::INTEGER[])
			ON CONFLICT (token_id)
			DO UPDATE
			SET index = array_append(tokenchain_index.index, $2),
				updated_at = NOW();
		`, returnedTokenID, returnedID); err != nil {
			return fmt.Errorf("PersistPreConsensus: failed to update token chain index for burnt token ID %s, err: %v", tokenID, err)
		}
	}

	for _, genesisRecord := range req.GenesisTokens {
		if _, err := tx.Exec(w.Ctx,
			`INSERT INTO tokens (token_id, parent_token_id, token_value, token_status, did, token_state_hash, transaction_id, token_type, latest_role, latest_position, created_at, updated_at) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			genesisRecord.Token.TokenID, genesisRecord.Token.ParentTokenID.String, genesisRecord.Token.TokenValue,
			constants.TokenStatus_Free, genesisRecord.Token.DID, "", genesisRecord.Token.TransactionID,
			genesisRecord.Token.TokenType, models.GetTokenRoleID(constants.TokenRole_Mint), 0, time.Now(), time.Now(),
		); err != nil {
			return fmt.Errorf("PersistPreConsensus: failed to insert child token %s, err: %v", genesisRecord.Token.TokenID, err)
		}

		// Insert token chain entries for the child tokens
		var tokenIDval string
		var rowID int

		row := tx.QueryRow(w.Ctx,
			`
			INSERT INTO tokenchain (
				token_id,
				transaction_id,
				previous_transaction_id,
				role,
				position
			)
			VALUES (
				$1,
				$2,
				(
					SELECT transaction_id
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				),
				$3,
				COALESCE((
					SELECT position
					FROM tokenchain
					WHERE token_id = $1
					ORDER BY position DESC
					LIMIT 1
				), -1) + 1
			) RETURNING token_id, id`,
			genesisRecord.TokenChain.TokenID, req.GenesisTransaction.ID, models.GetTokenRoleID(constants.TokenRole_Mint),
		)
		if err := row.Scan(&tokenIDval, &rowID); err != nil {
			return fmt.Errorf("PersistPreConsensus: failed to insert token chain entry for child token ID %s, err: %v", genesisRecord.TokenChain.TokenID, err)
		}

		// Insert token chain index entry for the child tokens
		if _, err := tx.Exec(w.Ctx, `
			INSERT INTO tokenchain_index (token_id, index)
			VALUES ($1, ARRAY[$2]::INTEGER[])
		`, tokenIDval, rowID); err != nil {
			return fmt.Errorf("PersistPreConsensus: unexpected error: token record for %s already exists, cannot insert token chain index, err: %v", genesisRecord.TokenChain.TokenID, err)
		}
	}

	// Update denom array in `token_denom` table
	for denomValue, delta := range denomArrayDelta {
		if _, err := tx.Exec(w.Ctx,
			`INSERT INTO token_denom (did, denom, count) VALUES ($1, $2, $3)
			ON CONFLICT (did, denom) DO UPDATE SET count = token_denom.count + $3, updated_at = EXCLUDED.updated_at
			`, req.DID, denomValue, delta,
		); err != nil {
			return fmt.Errorf("PersistPreConsensus: failed to update token_denom for denom value %f, err: %v", denomValue, err)
		}
	}

	if err := tx.Commit(w.Ctx); err != nil {
		return fmt.Errorf("PersistPreConsensus: failed to commit transaction, err: %v", err)
	}

	return nil
}
