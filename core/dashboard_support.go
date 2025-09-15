package core

import (
	"fmt"
	"strings"
	"time"
)

// Dashboard support structures
type PledgedTokenInfo struct {
	TokenID       string    `json:"token_id"`
	TokenType     int       `json:"token_type"`
	TokenValue    float64   `json:"token_value"`
	ReceiverDID   string    `json:"receiver_did"`
	TransactionID string    `json:"transaction_id"`
	PledgedAt     time.Time `json:"pledged_at"`
	DID           string    `json:"did"`
}

type UnpledgeSequenceInfo struct {
	TransactionID string    `json:"transaction_id"`
	TokenIDs      []string  `json:"token_ids"`
	InitiatedAt   time.Time `json:"initiated_at"`
	Status        string    `json:"status"`
	QuorumDID     string    `json:"quorum_did"`
	Epoch         int64     `json:"epoch"`
}

type TransactionInfo struct {
	TransactionID   string    `json:"transaction_id"`
	TransactionType string    `json:"transaction_type"`
	Amount          float64   `json:"amount"`
	SenderDID       string    `json:"sender_did"`
	ReceiverDID     string    `json:"receiver_did"`
	DateTime        time.Time `json:"date_time"`
	Status          string    `json:"status"`
	BlockID         string    `json:"block_id"`
}

// GetAllDIDs returns all DIDs for the current node
func (c *Core) GetAllDIDs() []string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	var dids []string

	// Get all DIDs from the wallet
	w := c.w
	if w != nil {
		didList, err := w.GetAllDIDs()
		if err != nil {
			c.log.Error("Failed to get DIDs from wallet", "err", err)
			return dids
		}

		for _, did := range didList {
			dids = append(dids, did.DID)
		}
	}

	// If no DIDs found in wallet, try to get from peer ID
	if len(dids) == 0 && c.peerID != "" {
		dids = append(dids, c.peerID)
	}

	return dids
}

// GetTokensByDID returns all tokens for a specific DID
func (c *Core) GetTokensByDID(did string) ([]interface{}, error) {
	if did == "" {
		return nil, fmt.Errorf("DID is required")
	}

	// Get tokens from wallet
	w := c.w
	if w == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	// Get all tokens for the DID (pass 0 to get all tokens)
	tokens, err := w.GetTokens(did, 0)
	if err != nil {
		c.log.Error("Failed to get tokens", "did", did, "err", err)
		return nil, err
	}

	// Convert to interface array
	var result []interface{}
	for _, token := range tokens {
		result = append(result, map[string]interface{}{
			"token_id":     token.TokenID,
			"token_type":   0, // Default type, can be determined from token ID prefix
			"token_value":  token.TokenValue,
			"token_status": token.TokenStatus,
			"did":          token.DID,
		})
	}

	return result, nil
}

// GetPledgedTokenCount returns the count of pledged tokens for a DID
func (c *Core) GetPledgedTokenCount(did string) (int, error) {
	if did == "" {
		return 0, fmt.Errorf("DID is required")
	}

	// Get pledged tokens
	pledgedTokens, err := c.GetPledgedTokens(did)
	if err != nil {
		return 0, err
	}

	return len(pledgedTokens), nil
}

// GetUnpledgeQueueInfo returns unpledge queue information for a DID
func (c *Core) GetUnpledgeQueueInfo(did string) (interface{}, error) {
	if did == "" {
		return nil, fmt.Errorf("DID is required")
	}

	// Get unpledge sequences
	sequences, err := c.GetUnpledgeSequences(did)
	if err != nil {
		return nil, err
	}

	// Calculate total tokens in unpledge queue
	totalTokens := 0
	for _, seq := range sequences {
		totalTokens += len(seq.TokenIDs)
	}

	return map[string]interface{}{
		"total_sequences": len(sequences),
		"total_tokens":    totalTokens,
		"sequences":       sequences,
	}, nil
}

// GetTransactionHistory returns transaction history for a DID
func (c *Core) GetTransactionHistory(did string, limit int) ([]TransactionInfo, error) {
	if did == "" {
		return nil, fmt.Errorf("DID is required")
	}

	var transactions []TransactionInfo

	// Get transaction history from database
	w := c.w
	if w == nil {
		return transactions, fmt.Errorf("wallet not initialized")
	}

	// Get actual transaction details from the transaction storage
	txDetails, err := w.GetTransactionByDID(did)
	if err != nil {
		c.log.Info("No regular transactions found for DID", "did", did, "err", err)
		// Continue to check FT transactions
	} else {
		// Convert transaction details to TransactionInfo
		for _, tx := range txDetails {
			transactions = append(transactions, TransactionInfo{
				TransactionID:   tx.TransactionID,
				TransactionType: tx.TransactionType,
				Amount:          tx.Amount,
				SenderDID:       tx.SenderDID,
				ReceiverDID:     tx.ReceiverDID,
				DateTime:        tx.DateTime,
				Status: func() string {
					if tx.Status {
						return "completed"
					} else {
						return "failed"
					}
				}(),
				BlockID: tx.BlockID,
			})

			if len(transactions) >= limit {
				return transactions, nil
			}
		}
	}

	// Also get FT transaction history
	ftTxDetails, err := w.GetAllFTTransactionDetailsByDID(did)
	if err != nil {
		c.log.Info("No FT transactions found for DID", "did", did, "err", err)
	} else {
		// Convert FT transaction details to TransactionInfo
		for _, tx := range ftTxDetails {
			transactions = append(transactions, TransactionInfo{
				TransactionID:   tx.TransactionID,
				TransactionType: tx.TransactionType,
				Amount:          tx.Amount,
				SenderDID:       tx.SenderDID,
				ReceiverDID:     tx.ReceiverDID,
				DateTime:        tx.DateTime,
				Status: func() string {
					if tx.Status {
						return "completed"
					} else {
						return "failed"
					}
				}(),
				BlockID: tx.BlockID,
			})

			if len(transactions) >= limit {
				break
			}
		}
	}

	return transactions, nil
}

// GetPledgedTokens returns pledged tokens for a DID
func (c *Core) GetPledgedTokens(did string) ([]PledgedTokenInfo, error) {
	if did == "" {
		return nil, fmt.Errorf("DID is required")
	}

	var pledgedTokens []PledgedTokenInfo

	// Get wallet instance
	w := c.w
	if w == nil {
		return pledgedTokens, fmt.Errorf("wallet not initialized")
	}

	// Get all pledged tokens from wallet
	tokens, err := w.GetAllPledgedTokens()
	if err != nil {
		c.log.Error("Failed to get pledged tokens", "err", err)
		return pledgedTokens, err
	}

	// Filter by DID if specified, otherwise return all
	for _, token := range tokens {
		if did == "" || token.DID == did {
			pledgedTokens = append(pledgedTokens, PledgedTokenInfo{
				TokenID:       token.TokenID,
				TokenType:     0, // Default type, can be determined from token ID prefix
				TokenValue:    token.TokenValue,
				DID:           token.DID,
				TransactionID: token.TransactionID,
				PledgedAt:     token.UpdatedAt,
			})
		}
	}

	return pledgedTokens, nil
}

// GetUnpledgeSequences returns unpledge sequences for a DID
func (c *Core) GetUnpledgeSequences(did string) ([]UnpledgeSequenceInfo, error) {
	var sequences []UnpledgeSequenceInfo

	// Get wallet instance
	w := c.w
	if w == nil {
		return sequences, fmt.Errorf("wallet not initialized")
	}

	// Get unpledge sequence details from wallet
	unpledgeData, err := w.GetUnpledgeSequenceDetails()
	if err != nil {
		c.log.Info("Failed to get unpledge sequences", "did", did, "err", err)
		return sequences, nil // Return empty array instead of error
	}

	// Convert to UnpledgeSequenceInfo
	for _, seq := range unpledgeData {
		// Parse pledge tokens string to get token IDs
		tokenIDs := []string{}
		if seq.PledgeTokens != "" {
			tokenIDs = strings.Split(seq.PledgeTokens, ",")
		}

		// Filter by DID if needed (check if QuorumDID matches or if it's in token list)
		if did == "" || seq.QuorumDID == did {
			sequences = append(sequences, UnpledgeSequenceInfo{
				TransactionID: seq.TransactionID,
				TokenIDs:      tokenIDs,
				InitiatedAt:   time.Unix(seq.Epoch, 0),
				Status:        "pending",
				QuorumDID:     seq.QuorumDID,
				Epoch:         seq.Epoch,
			})
		}
	}

	return sequences, nil
}
