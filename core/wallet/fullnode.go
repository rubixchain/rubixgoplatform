package wallet

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func (w *Wallet) GetFullNodeRBTToken(tokenID string) (models.FullNodeRBT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, parent_token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, latest_position, latest_role, created_at, updated_at
		 FROM fullnode_rbt WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeRBT
	if err := row.Scan(
		&t.TokenID, &t.ParentTokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeRBT{}, fmt.Errorf("GetFullNodeRBTToken: token %v not found", tokenID)
		}
		return models.FullNodeRBT{}, fmt.Errorf("GetFullNodeRBTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeFTToken(tokenID string) (models.FullNodeFT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, did, transaction_id,
		 token_state_hash, latest_position, latest_role, created_at, updated_at
		 FROM fullnode_ft WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeFT
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeFT{}, fmt.Errorf("GetFullNodeFTToken: token %v not found", tokenID)
		}
		return models.FullNodeFT{}, fmt.Errorf("GetFullNodeFTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeNFTToken(tokenID string) (models.FullNodeNFT, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, did, transaction_id,
		 token_state_hash,latest_position, latest_role, created_at, updated_at
		 FROM fullnode_nft WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeNFT
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.DID, &t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeNFT{}, fmt.Errorf("GetFullNodeNFTToken: token %v not found", tokenID)
		}
		return models.FullNodeNFT{}, fmt.Errorf("GetFullNodeNFTToken scan: %w", err)
	}

	return t, nil
}

func (w *Wallet) GetFullNodeSmartContractToken(tokenID string) (models.FullNodeSmartContract, error) {
	row := w.db.Pool().QueryRow(w.Ctx,
		`SELECT token_id, token_value, token_status, transaction_id,
		 token_state_hash,  latest_position, latest_role, created_at, updated_at
		 FROM fullnode_smart_contract WHERE token_id = $1`, tokenID,
	)

	var t models.FullNodeSmartContract
	if err := row.Scan(
		&t.TokenID, &t.TokenValue, &t.TokenStatus,
		&t.TransactionID, &t.TokenStateHash, 
		&t.LatestPosition, &t.LatestRole, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return models.FullNodeSmartContract{}, fmt.Errorf("GetFullNodeSmartContractToken: token %v not found", tokenID)
		}
		return models.FullNodeSmartContract{}, fmt.Errorf("GetFullNodeSmartContractToken scan: %w", err)
	}

	return t, nil
}
