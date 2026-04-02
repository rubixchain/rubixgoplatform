package wallet

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
)

// AddUnpledgeSequenceInfo persists an unpledge sequence record.
func (w *Wallet) AddUnpledgeSequenceInfo(info *UnpledgeSequenceInfo) error {
	// TODO(phase07): insert into unpledge_sequence table
	return nil
}

// GetCredit returns credit records for a DID.
func (w *Wallet) GetCredit(did string) ([]string, error) {
	// TODO(phase07): query credit table for DID
	return nil, nil
}

// TokensReceived updates token ownership when RBT tokens are received.
func (w *Wallet) TokensReceived(
	receiverDID string,
	tokenInfo interface{},
	blk interface{},
	senderPeerID string,
	receiverPeerID string,
	pinningServiceMode bool,
) ([]string, error) {
	// TODO(phase07): update token owner/status in tokens table
	return nil, nil
}

// FTTokensReceived updates FT token ownership when FT tokens are received.
func (w *Wallet) FTTokensReceived(
	receiverDID string,
	tokenInfo interface{},
	blk interface{},
	senderPeerID string,
	receiverPeerID string,
	ft FTToken,
) ([]string, error) {
	// TODO(phase07): update FT token owner/status in tokens table
	return nil, nil
}

// ReadFTToken returns the FT token record for a given token ID.
func (w *Wallet) ReadFTToken(tokenID string) (*FTToken, error) {
	// TODO(phase07): query ft tokens table by token_id
	return nil, nil
}

// PledgeWholeToken records a pledge operation for a token.
func (w *Wallet) PledgeWholeToken(did string, token string, blk interface{}) error {
	// TODO(phase07): update token status to pledged in tokens table
	return nil
}

// AddTokenStateHash records a token state hash mapping.
func (w *Wallet) AddTokenStateHash(did string, hashes []string, pledgedTokens []string, txID string) error {
	// TODO(phase07): insert into token_state_hashes table
	return nil
}

// UnlockLockedTokens releases specific locked tokens for a DID back to Free status.
// Called by the /api/unlock-tokens quorum-side endpoint when a transaction is aborted.
func (w *Wallet) UnlockLockedTokens(did string, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1, updated_at=$2
		 WHERE did=$3 AND token_id = ANY($4::text[]) AND token_status=$5`,
		constants.TokenStatus_Free, time.Now(), did, tokens, constants.TokenStatus_Locked,
	)
	return err
}

// RemoveTokenStateHash removes a single token state hash record.
func (w *Wallet) RemoveTokenStateHash(tokenStateHash string) error {
	// TODO(phase07): delete from token_state_hashes where hash=$1
	return nil
}
