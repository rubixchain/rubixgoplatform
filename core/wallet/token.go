package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) GetRBTTokens() ([]models.Token, error) {
	return w.queryTokensByType(constants.TokenType_RBT)
}

func (w *Wallet) GetFTTokens() ([]models.Token, error) {
	return w.queryTokensByType(constants.TokenType_FT)
}

func (w *Wallet) GetNFTTokens() ([]models.Token, error) {
	return w.queryTokensByType(constants.TokenType_NFT)
}

func (w *Wallet) GetSmartContractTokens() ([]models.Token, error) {
	return w.queryTokensByType(constants.TokenType_SmartContract)
}

// GetFreeRBTTokens returns tokens along with their IDs
func (w *Wallet) GetFreeRBTTokens(ownerDid string) ([]models.Token, []string, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id, token_state_hash, token_type, latest_position, latest_role, created_at, updated_at FROM tokens WHERE token_type = (
			SELECT id
			FROM token_type
			WHERE name = $1
		) AND did = $2 AND token_status = $3
		`, constants.TokenType_RBT, ownerDid, constants.TokenStatus_Free,
	)
	if err != nil {
		return nil, nil, err
	}

	var freeTokens []models.Token
	var freeTokenIDs []string
	for rows.Next() {
		var freeToken models.Token
		err := rows.Scan(
			&freeToken.TokenID, &freeToken.ParentTokenID, &freeToken.TokenValue, &freeToken.TokenStatus,
			&freeToken.DID, &freeToken.TransactionID, &freeToken.TokenStateHash, &freeToken.TokenType,
			&freeToken.LatestPosition, &freeToken.LatestRole,
			&freeToken.CreatedAt, &freeToken.UpdatedAt,
		)
		if err != nil {
			return nil, nil, err
		}
		freeTokens = append(freeTokens, freeToken)
		freeTokenIDs = append(freeTokenIDs, freeToken.TokenID)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return freeTokens, freeTokenIDs, nil
}

func (w *Wallet) GetTokenByTokenID(tokenID string) (models.Token, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE token_id=$1`, tokenID,
	)

	var token models.Token
	if err := row.Scan(
		&token.TokenID, &token.ParentTokenID, &token.TokenValue, &token.TokenStatus,
		&token.DID, &token.TransactionID, &token.TokenStateHash, &token.TokenType,
		&token.LatestPosition, &token.LatestRole, &token.CreatedAt, &token.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.Token{}, fmt.Errorf("GetTokenByTokenID: token with id %v not found", tokenID)
		}
		return models.Token{}, fmt.Errorf("GetTokenByTokenID scan: %w", err)
	}

	return token, nil
}

func (w *Wallet) GetRBTTokenByStatus(tokenID string, tokenStatus int) (models.Token, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE token_id=$1 AND token_status=$2`, tokenID, tokenStatus,
	)

	var rbtToken models.Token
	if err := row.Scan(
		&rbtToken.TokenID, &rbtToken.ParentTokenID, &rbtToken.TokenValue, &rbtToken.TokenStatus,
		&rbtToken.DID, &rbtToken.TransactionID, &rbtToken.TokenStateHash, &rbtToken.TokenType,
		&rbtToken.LatestPosition, &rbtToken.LatestRole, &rbtToken.CreatedAt, &rbtToken.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.Token{}, fmt.Errorf("GetRBTToken: token with id %v not found", tokenID)
		}
		return models.Token{}, fmt.Errorf("GetLatestTransactionByTokenID scan: %w", err)
	}

	return rbtToken, nil
}

func (w *Wallet) LockTokensByID(tokenIDs []string) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1 where token_id = ANY($2)`, constants.TokenStatus_Locked, tokenIDs,
	); err != nil {
		return fmt.Errorf("unable to lock tokens, err: %v", err)
	}

	return nil
}

func (w *Wallet) LockTokenByID(tokenID string) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1 where token_id = $2`, constants.TokenStatus_Locked, tokenID,
	); err != nil {
		return fmt.Errorf("unable to lock tokens, err: %v", err)
	}

	return nil
}

func (w *Wallet) LockTokens(tokens []models.Token) error {
	var tokenIds []string
	for _, token := range tokens {
		tokenIds = append(tokenIds, token.TokenID)
	}

	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1 where token_id = ANY($2)`, constants.TokenStatus_Locked, tokenIds,
	); err != nil {
		return fmt.Errorf("unable to lock tokens, err: %v", err)
	}

	return nil
}

func (w *Wallet) LockToken(token models.Token) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1 where token_id = $2`, constants.TokenStatus_Locked, token.TokenID,
	); err != nil {
		return fmt.Errorf("unable to lock tokens, err: %v", err)
	}

	return nil
}

func (w *Wallet) BurnToken(tokenID string) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1 where token_id = $2`, constants.TokenStatus_Burnt, tokenID,
	); err != nil {
		return fmt.Errorf("unable to lock tokens, err: %v", err)
	}

	return nil
}

func (w *Wallet) queryTokensByType(tokenType string) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id, token_state_hash, token_type, latest_position, latest_role, created_at, updated_at FROM tokens WHERE token_type = (
			SELECT id
			FROM token_type
			WHERE name = $1
		)
		`,
		tokenType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.Token
	for rows.Next() {
		var token models.Token
		err := rows.Scan(
			&token.TokenID, &token.ParentTokenID, &token.TokenValue,
			&token.TokenStatus,
			&token.DID, &token.TransactionID, &token.TokenStateHash, &token.TokenType,
			&token.LatestPosition, &token.LatestRole,
			&token.CreatedAt, &token.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("GetRBTTokens: error occured while scanning rows: %v", err)
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetRBTTokens: error occured while streaming RBT token info, err: %v", err)
	}

	return tokens, nil
}

func (w *Wallet) GetTokensFromDenomMap(denomMap map[types.DenomValue]types.DenomCount, did string) ([]models.Token, error) {
	var tokens []models.Token = make([]models.Token, 0)

	for denomValue, denomCount := range denomMap {
		if denomCount == 0 {
			continue
		}

		rows, _ := w.db.Pool().Query(w.Ctx,
			`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
			 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
			 FROM tokens WHERE token_value=$1 AND did=$2 AND token_status=$3 LIMIT $4`,
			denomValue,
			did,
			constants.TokenStatus_Free,
			denomCount,
		)
		for rows.Next() {
			var rbtToken models.Token
			if err := rows.Scan(
				&rbtToken.TokenID, &rbtToken.ParentTokenID, &rbtToken.TokenValue, &rbtToken.TokenStatus,
				&rbtToken.DID, &rbtToken.TransactionID, &rbtToken.TokenStateHash, &rbtToken.TokenType,
				&rbtToken.LatestPosition, &rbtToken.LatestRole, &rbtToken.CreatedAt, &rbtToken.UpdatedAt,
			); err != nil {
				return nil, fmt.Errorf("GetTokensFromDenomMap scan: %w", err)
			}

			tokens = append(tokens, rbtToken)
		}
	}

	return tokens, nil
}

func (w *Wallet) UpdateToken(token models.Token) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET did=$1, transaction_id=$2, token_state_hash=$3,
		 token_status=$4, latest_position=$5, latest_role=$6, updated_at=NOW()
		 WHERE token_id=$7`,
		token.DID, token.TransactionID, token.TokenStateHash,
		token.TokenStatus, token.LatestPosition, token.LatestRole,
		token.TokenID,
	); err != nil {
		return fmt.Errorf("failed to update token %v: %w", token.TokenID, err)
	}

	return nil
}

func (w *Wallet) GetRBTTokensChunk(did string, limit, offset int) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1 AND token_type=(SELECT id FROM token_type WHERE name=$2)
		 LIMIT $3 OFFSET $4`,
		did, constants.TokenType_RBT, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("GetRBTTokensChunk: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetRBTTokensChunk scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (w *Wallet) GetFTTokensChunk(did string, limit, offset int) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1 AND token_type=(SELECT id FROM token_type WHERE name=$2)
		 LIMIT $3 OFFSET $4`,
		did, constants.TokenType_FT, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("GetFTTokensChunk: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetFTTokensChunk scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (w *Wallet) GetSmartContractTokensChunk(did string, limit, offset int) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1 AND token_type=(SELECT id FROM token_type WHERE name=$2)
		 LIMIT $3 OFFSET $4`,
		did, constants.TokenType_SmartContract, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("GetSmartContractTokensChunk: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetSmartContractTokensChunk scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (w *Wallet) GetAllPinnedTokens(did string) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1 AND token_status=$2`,
		did, constants.TokenStatus_PinnedAsService,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllPinnedTokens: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetAllPinnedTokens scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// ReleaseAllLockedRBTTokensForDID resets all Locked RBT tokens for a DID back to Free.
// Called on transaction failure after LockTokensForSplit to prevent tokens from staying
// permanently locked when the transaction does not complete.
func (w *Wallet) ReleaseAllLockedRBTTokensForDID(ctx context.Context, ownerDID string) error {
	_, err := w.db.Pool().Exec(ctx,
		`UPDATE tokens SET token_status=$1, updated_at=$2
		 WHERE did=$3
		   AND token_status=$4
		   AND token_type=(SELECT id FROM token_type WHERE name=$5)`,
		constants.TokenStatus_Free, time.Now(), ownerDID, constants.TokenStatus_Locked, constants.TokenType_RBT,
	)
	return err
}

// ReleaseNonSelectedLockedRBTTokensForDID resets all Locked RBT tokens for a DID back to Free,
// EXCLUDING the specified selectedTokenIDs. Used by the quorum pledge handler to release
// candidate tokens that were locked by LockTokensForSplit but not chosen for the pledge,
// while keeping the selected pledge tokens Locked so PledgeTokens can transition them to Pledged.
func (w *Wallet) ReleaseNonSelectedLockedRBTTokensForDID(ctx context.Context, ownerDID string, selectedTokenIDs []string) error {
	_, err := w.db.Pool().Exec(ctx,
		`UPDATE tokens SET token_status=$1, updated_at=$2
		 WHERE did=$3
		   AND token_status=$4
		   AND token_type=(SELECT id FROM token_type WHERE name=$5)
		   AND token_id != ALL($6::text[])`,
		constants.TokenStatus_Free, time.Now(), ownerDID, constants.TokenStatus_Locked, constants.TokenType_RBT, selectedTokenIDs,
	)
	return err
}

// ReleaseTokens sets the status of a slice of tokens back to Free (unlocked).
// It accepts the same []*models.TokenInfo slice returned by CollectRBTTokens.
// This is a best-effort operation: errors are logged but do not abort the loop.
func (w *Wallet) ReleaseTokens(tokens []*models.TokenInfo) {
	for _, t := range tokens {
		if t == nil || t.TokenID == "" {
			continue
		}
		if _, err := w.db.Pool().Exec(w.Ctx,
			`UPDATE tokens SET token_status=$1 WHERE token_id=$2`,
			constants.TokenStatus_Free, t.TokenID,
		); err != nil {
			// Log but do not propagate — release is best-effort
			_ = fmt.Errorf("ReleaseTokens: failed to release token %s: %w", t.TokenID, err)
		}
	}
}

func (w *Wallet) IsDIDExist(did string) bool {
	var exists bool
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT EXISTS(SELECT 1 FROM dids WHERE did=$1)`, did,
	).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func (w *Wallet) ReadToken(tokenID string) (*models.Token, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE token_id=$1`, tokenID,
	)
	var t models.Token
	if err := row.Scan(
		&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ReadToken: token %v not found", tokenID)
		}
		return nil, fmt.Errorf("ReadToken scan: %w", err)
	}
	return &t, nil
}

func (w *Wallet) GetAllTokens(did string) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1`, did,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllTokens: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetAllTokens scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (w *Wallet) CreateToken(t *models.Token) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO tokens(token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		t.TokenID, t.ParentTokenID, t.TokenValue, t.TokenStatus,
		t.DID, t.TransactionID, t.TokenStateHash, t.TokenType,
		t.LatestPosition, t.LatestRole, t.CreatedAt, t.UpdatedAt,
	); err != nil {
		return fmt.Errorf("CreateToken: %w", err)
	}
	return nil
}

func (w *Wallet) CreateRBTToken(token models.Token) error {
	if _, err := w.db.Pool().Exec(w.Ctx,
		`INSERT INTO tokens(token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		token.TokenID, token.ParentTokenID, token.TokenValue, token.TokenStatus,
		token.DID, token.TransactionID, token.TokenStateHash, token.TokenType,
		token.LatestPosition, token.LatestRole, token.CreatedAt, token.UpdatedAt,
	); err != nil {
		return fmt.Errorf("failed to create token with id: %v, err: %v", token.TokenID, err)
	}

	return nil
}

// IsSmartContract checks if the given token ID is of type smart_contract.
// Returns true if the token exists and is a smart contract, false otherwise.
func (w *Wallet) IsSmartContract(tokenID string) (bool, error) {
	var exists bool
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT EXISTS(
			SELECT 1 FROM tokens t
			JOIN token_type tt ON t.token_type = tt.id
			WHERE t.token_id = $1 AND tt.name = $2
		)`,
		tokenID, constants.TokenType_SmartContract,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("IsSmartContract: failed to check token type: %w", err)
	}

	return exists, nil
}

// IsNFT checks if the given token ID is of type nft.
// Returns true if the token exists and is an NFT, false otherwise.
func (w *Wallet) IsNFT(tokenID string) (bool, error) {
	var exists bool
	err := w.db.Pool().QueryRow(w.Ctx,
		`SELECT EXISTS(
			SELECT 1 FROM tokens t
			JOIN token_type tt ON t.token_type = tt.id
			WHERE t.token_id = $1 AND tt.name = $2
		)`,
		tokenID, constants.TokenType_NFT,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("IsNFT: failed to check token type: %w", err)
	}

	return exists, nil
}

func (w *Wallet) GetTokenByDIDAndTokenType(didStr string, tokenType int16) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, token_type, latest_position, latest_role, created_at, updated_at
		 FROM tokens WHERE did=$1 AND token_type=$2`, didStr, tokenType,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllTokens: %w", err)
	}
	defer rows.Close()
	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(
			&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
			&t.DID, &t.TransactionID, &t.TokenStateHash, &t.TokenType,
			&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetAllTokens scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (w *Wallet) IsRBTExists(id string) bool {
	var exists bool
	_ = w.db.Pool().QueryRow(w.Ctx,
		`SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=$1 AND token_type=$2)`, 
		id,
		models.GetTokenTypeID(constants.TokenType_RBT),
	).Scan(&exists)
	return exists
}