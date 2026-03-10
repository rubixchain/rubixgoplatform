package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
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

func (w *Wallet) GetFreeRBTTokens(ownerDid string) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT * FROM tokens WHERE token_type = (
			SELECT id
			FROM token_type
			WHERE name = $1 
		) AND did = $2 AND token_status = 0
		`, constants.TokenType_RBT, ownerDid,
	)
	if err != nil {
		return nil, err
	}

	var freeTokens []models.Token
	for rows.Next() {
		var freeToken models.Token
		err := rows.Scan(
			&freeToken.TokenID, &freeToken.ParentTokenID, &freeToken.TokenValue, &freeToken.TokenStatus,
			&freeToken.DID, &freeToken.TransactionID, &freeToken.TokenStateHash, &freeToken.TokenType,
			&freeToken.CreatedAt, &freeToken.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		freeTokens = append(freeTokens, freeToken)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return freeTokens, nil
}

func (w *Wallet) queryTokensByType(tokenType string) ([]models.Token, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT * FROM tokens WHERE token_type = (
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
			&token.TokenStatus, //missed to add token status. expecting 10 but only sharing 9. resolved
			&token.DID, &token.TransactionID, &token.TokenStateHash, &token.TokenType,
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
