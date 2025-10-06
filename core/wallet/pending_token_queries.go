package wallet

import (
	"time"
)

// GetPendingFTTokensOlderThan returns FT tokens that have been pending for longer than the specified duration
// Returns a map of transaction ID to token IDs
func (w *Wallet) GetPendingFTTokensOlderThan(duration time.Duration) (map[string][]string, error) {
	w.l.Lock()
	defer w.l.Unlock()
	
	// Query for pending FT tokens older than the specified duration
	cutoffTime := time.Now().Add(-duration)
	var pendingTokens []FTToken
	err := w.s.Read(FTTokenStorage, &pendingTokens, 
		"token_status = ? AND created_at < ?", 
		TokenIsPending, cutoffTime)
	
	if err != nil {
		// No pending tokens found is not an error
		if err.Error() == "no records found" {
			return make(map[string][]string), nil
		}
		return nil, err
	}
	
	// Group by transaction ID
	result := make(map[string][]string)
	for _, token := range pendingTokens {
		if token.TransactionID != "" {
			result[token.TransactionID] = append(result[token.TransactionID], token.TokenID)
		}
	}
	
	return result, nil
}

// GetPendingTokensOlderThan returns RBT tokens that have been pending for longer than the specified duration
// Returns a map of transaction ID to token IDs
func (w *Wallet) GetPendingTokensOlderThan(duration time.Duration) (map[string][]string, error) {
	w.l.Lock()
	defer w.l.Unlock()
	
	// Query for pending tokens older than the specified duration
	cutoffTime := time.Now().Add(-duration)
	var pendingTokens []Token
	err := w.s.Read(TokenStorage, &pendingTokens, 
		"token_status = ? AND created_at < ?", 
		TokenIsPending, cutoffTime)
	
	if err != nil {
		// No pending tokens found is not an error
		if err.Error() == "no records found" {
			return make(map[string][]string), nil
		}
		return nil, err
	}
	
	// Group by transaction ID
	result := make(map[string][]string)
	for _, token := range pendingTokens {
		if token.TransactionID != "" {
			result[token.TransactionID] = append(result[token.TransactionID], token.TokenID)
		}
	}
	
	return result, nil
}