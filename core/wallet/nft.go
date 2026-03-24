package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// NFT is a legacy stub type for NFT tokens.
type NFT struct {
	TokenID     string
	DID         string
	TokenStatus int
	TokenValue  float64
	Metadata    string
	Filename    string
}

// GetNFTToken returns the NFT token record for a given token ID.
func (w *Wallet) GetNFTToken(tokenID string) (*NFT, error) {
	// TODO(phase07): query tokens table where token_id=$1 and token_type=nft
	return nil, fmt.Errorf("NFT token not found: %s", tokenID)
}

// GetAllNFT returns all NFT tokens stored in the wallet DB.
func (w *Wallet) GetAllNFT() ([]NFT, error) {
	// TODO(phase07): query tokens table where token_type=nft
	return nil, nil
}

// GetNFTsByDid returns all NFT tokens owned by the given DID.
func (w *Wallet) GetNFTsByDid(did string) ([]NFT, error) {
	// TODO(phase07): query tokens table where did=$1 and token_type=nft
	return nil, nil
}

// GetSmartContractTokenUrl returns a URL for the smart contract token.
func (w *Wallet) GetSmartContractTokenUrl(tokenID string) (string, error) {
	// TODO(phase07): query smart contract storage for URL
	return "", nil
}

func (w *Wallet) UpdateNFTStatus(nftHash string, status int, local bool, receiver string, nftValue float64) error {
	if local {
		_, err := w.db.Pool().Exec(w.Ctx,
			`UPDATE tokens SET token_status=$1, token_value=$2, updated_at=NOW() WHERE token_id=$3`,
			status, nftValue, nftHash,
		)
		if err != nil {
			return fmt.Errorf("UpdateNFTStatus: %w", err)
		}
	} else {
		_, err := w.db.Pool().Exec(w.Ctx,
			`UPDATE tokens SET token_status=$1, did=$2, token_value=$3, updated_at=NOW() WHERE token_id=$4`,
			status, receiver, nftValue, nftHash,
		)
		if err != nil {
			return fmt.Errorf("UpdateNFTStatus: %w", err)
		}
	}
	return nil
}

func (w *Wallet) CreateNFT(nft *NFT, exists bool) error {
	if exists {
		_, err := w.db.Pool().Exec(w.Ctx,
			`UPDATE tokens SET token_status=$1, did=$2, token_value=$3, updated_at=NOW() WHERE token_id=$4`,
			nft.TokenStatus, nft.DID, nft.TokenValue, nft.TokenID,
		)
		if err != nil {
			return fmt.Errorf("CreateNFT update: %w", err)
		}
	} else {
		_, err := w.db.Pool().Exec(w.Ctx,
			`INSERT INTO tokens(token_id, token_value, token_status, did, transaction_id, token_state_hash, token_type, latest_position, latest_role, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, '', '', (SELECT id FROM token_type WHERE name=$5), 0, 0, NOW(), NOW())
			 ON CONFLICT(token_id) DO UPDATE SET token_status=EXCLUDED.token_status, did=EXCLUDED.did, token_value=EXCLUDED.token_value, updated_at=NOW()`,
			nft.TokenID, nft.TokenValue, nft.TokenStatus, nft.DID, constants.TokenType_NFT,
		)
		if err != nil {
			return fmt.Errorf("CreateNFT insert: %w", err)
		}
	}
	return nil
}

func (w *Wallet) IsNFTExists(nftHash string) bool {
	var exists bool
	_ = w.db.Pool().QueryRow(w.Ctx,
		`SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=$1)`, nftHash,
	).Scan(&exists)
	return exists
}

func (w *Wallet) GetNFTTokensChunk(did string, limit, offset int) ([]NFT, error) {
	rows, err := w.db.Pool().Query(w.Ctx,
		`SELECT token_id, did, token_status, token_value
		 FROM tokens WHERE did=$1 AND token_type=(SELECT id FROM token_type WHERE name=$2)
		 LIMIT $3 OFFSET $4`,
		did, constants.TokenType_NFT, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("GetNFTTokensChunk: %w", err)
	}
	defer rows.Close()
	var nfts []NFT
	for rows.Next() {
		var n NFT
		if err := rows.Scan(&n.TokenID, &n.DID, &n.TokenStatus, &n.TokenValue); err != nil {
			return nil, fmt.Errorf("GetNFTTokensChunk scan: %w", err)
		}
		nfts = append(nfts, n)
	}
	return nfts, rows.Err()
}
