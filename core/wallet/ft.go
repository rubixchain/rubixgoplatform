package wallet

import (
	"fmt"
	"time"
)

type FTToken struct {
	TokenID        string    `gorm:"column:token_id;primaryKey"`
	FTName         string    `gorm:"column:ft_name"`
	DID            string    `gorm:"column:owner_did"`
	CreatorDID     string    `gorm:"column:creator_did"`
	TokenStatus    int       `gorm:"column:token_status"`
	TokenValue     float64   `gorm:"column:token_value"`
	TokenStateHash string    `gorm:"column:token_state_hash"`
	TransactionID  string    `gorm:"column:transaction_id"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

type FT struct {
	ID         string `gorm:"column:id;primaryKey;autoIncrement"`
	FTName     string `gorm:"column:ft_name"`
	FTCount    int    `gorm:"column:ft_count"`
	CreatorDID string `gorm:"column:creator_did"`
}

// LockFTTokenForTransfer locks a token for transfer to prevent double spending
func (w *Wallet) LockFTTokenForTransfer(tokenID, transactionID string) error {
	var ftToken FTToken
	err := w.s.Read(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to get FT token: %v", err)
	}

	if ftToken.TokenStatus != TokenIsFree {
		return fmt.Errorf("token is not free for transfer, status: %d", ftToken.TokenStatus)
	}

	// Lock the token for transfer
	ftToken.TokenStatus = TokenIsInTransfer
	ftToken.TransactionID = transactionID

	err = w.s.Update(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to lock FT token for transfer: %v", err)
	}

	return nil
}

// UnlockFTTokenFromTransfer unlocks a token if transfer fails
func (w *Wallet) UnlockFTTokenFromTransfer(tokenID string) error {
	var ftToken FTToken
	err := w.s.Read(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to get FT token: %v", err)
	}

	if ftToken.TokenStatus != TokenIsInTransfer {
		return fmt.Errorf("token is not in transfer state, status: %d", ftToken.TokenStatus)
	}

	// Unlock the token back to free status
	ftToken.TokenStatus = TokenIsFree
	ftToken.TransactionID = ""

	err = w.s.Update(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to unlock FT token: %v", err)
	}

	return nil
}

// GetFTTokensByDID returns all FT tokens for a given DID
func (w *Wallet) GetFTTokensByDID(did string) ([]FTToken, error) {
	var ftTokens []FTToken
	err := w.s.Read(FTTokenStorage, &ftTokens, "owner_did=?", did)
	if err != nil {
		return nil, fmt.Errorf("failed to get FT tokens for DID %s: %v", did, err)
	}
	return ftTokens, nil
}

// MarkFTTokenAsInTransfer marks a token as in-transfer for tracking purposes
// This function works with tokens that are already locked by the existing FT system
func (w *Wallet) MarkFTTokenAsInTransfer(tokenID, transactionID string) error {
	var ftToken FTToken
	err := w.s.Read(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to get FT token: %v", err)
	}

	// Check if token is locked (existing system locks them)
	if ftToken.TokenStatus != TokenIsLocked {
		return fmt.Errorf("token is not locked, status: %d", ftToken.TokenStatus)
	}

	// Update to in-transfer status
	ftToken.TokenStatus = TokenIsInTransfer
	ftToken.TransactionID = transactionID

	err = w.s.Update(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark FT token as in-transfer: %v", err)
	}

	return nil
}

// MarkFTTokenAsTransferred marks a token as successfully transferred
func (w *Wallet) MarkFTTokenAsTransferred(tokenID, transactionID string) error {
	var ftToken FTToken
	err := w.s.Read(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to get FT token: %v", err)
	}

	if ftToken.TokenStatus != TokenIsInTransfer {
		return fmt.Errorf("token is not in transfer state, status: %d", ftToken.TokenStatus)
	}

	// Mark as transferred and preserve TransactionID for tracking
	ftToken.TokenStatus = TokenIsTransferred
	// Note: We keep the TransactionID for transaction tracking

	err = w.s.Update(FTTokenStorage, &ftToken, "token_id=?", tokenID)
	if err != nil {
		return fmt.Errorf("failed to mark FT token as transferred: %v", err)
	}

	w.log.Info("FT token marked as transferred",
		"token_id", tokenID,
		"transaction_id", transactionID,
		"status", TokenIsTransferred)

	return nil
}
