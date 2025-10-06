package wallet

import (
	"fmt"
	"sync"
	"time"
)

// OptimizedGetFreeTokens fetches and processes free tokens efficiently for any large amount
func (w *Wallet) OptimizedGetFreeTokens(did string, requiredCount float64) ([]Token, error) {
	startTime := time.Now()
	
	// Step 1: Batch fetch all free tokens
	var allFreeTokens []Token
	err := w.s.Read(TokenStorage, &allFreeTokens, "token_status=? AND did=?", TokenIsFree, did)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch free tokens: %v", err)
	}
	
	w.log.Info("Fetched free tokens", 
		"count", len(allFreeTokens), 
		"fetch_time", time.Since(startTime))
	
	if len(allFreeTokens) == 0 {
		return []Token{}, nil
	}
	
	// Step 2: Calculate how many tokens we need
	tokensNeeded := calculateTokensNeeded(allFreeTokens, requiredCount)
	if len(tokensNeeded) == 0 {
		return nil, fmt.Errorf("insufficient free tokens: have %d, need %.2f", 
			len(allFreeTokens), requiredCount)
	}
	
	w.log.Info("Tokens needed for transaction", 
		"required_value", requiredCount,
		"token_count", len(tokensNeeded))
	
	// Step 3: Batch lock tokens (single UPDATE query)
	lockStart := time.Now()
	err = w.batchLockTokens(tokensNeeded, did)
	if err != nil {
		return nil, fmt.Errorf("failed to lock tokens: %v", err)
	}
	
	w.log.Info("Locked tokens", 
		"count", len(tokensNeeded),
		"lock_time", time.Since(lockStart))
	
	// Step 4: Parallel validation (if needed)
	if len(tokensNeeded) > 1000 {
		validationStart := time.Now()
		err = w.parallelValidateTokens(tokensNeeded)
		if err != nil {
			// Rollback locks
			w.batchUnlockTokens(tokensNeeded, did)
			return nil, fmt.Errorf("token validation failed: %v", err)
		}
		w.log.Info("Validated tokens", 
			"count", len(tokensNeeded),
			"validation_time", time.Since(validationStart))
	}
	
	w.log.Info("Total token fetch operation", 
		"token_count", len(tokensNeeded),
		"total_time", time.Since(startTime))
	
	return tokensNeeded, nil
}

// batchLockTokens locks multiple tokens efficiently
func (w *Wallet) batchLockTokens(tokens []Token, did string) error {
	if len(tokens) == 0 {
		return nil
	}
	
	// Process in chunks to avoid too many individual updates
	chunkSize := 100
	for i := 0; i < len(tokens); i += chunkSize {
		end := i + chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}
		
		chunk := tokens[i:end]
		// Lock tokens in parallel within chunk
		var wg sync.WaitGroup
		errChan := make(chan error, len(chunk))
		
		for j := range chunk {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				t := &chunk[idx]
				t.TokenStatus = TokenIsLocked
				err := w.s.Update(TokenStorage, t, "did=? AND token_id=?", did, t.TokenID)
				if err != nil {
					errChan <- fmt.Errorf("failed to lock token %s: %v", t.TokenID, err)
				}
			}(j)
		}
		
		wg.Wait()
		close(errChan)
		
		// Check for errors
		for err := range errChan {
			if err != nil {
				return err
			}
		}
	}
	
	return nil
}


// batchUnlockTokens unlocks multiple tokens (for rollback)
func (w *Wallet) batchUnlockTokens(tokens []Token, did string) error {
	// Process in parallel for efficiency
	var wg sync.WaitGroup
	errChan := make(chan error, len(tokens))
	
	for i := range tokens {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			t := &tokens[idx]
			t.TokenStatus = TokenIsFree
			err := w.s.Update(TokenStorage, t, "did=? AND token_id=?", did, t.TokenID)
			if err != nil {
				errChan <- fmt.Errorf("failed to unlock token %s: %v", t.TokenID, err)
			}
		}(i)
	}
	
	wg.Wait()
	close(errChan)
	
	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// parallelValidateTokens validates tokens in parallel for very large sets
func (w *Wallet) parallelValidateTokens(tokens []Token) error {
	if len(tokens) < 1000 {
		// For smaller sets, sequential is fine
		return nil
	}
	
	numWorkers := 10
	if len(tokens) > 10000 {
		numWorkers = 20
	}
	
	jobs := make(chan Token, len(tokens))
	errors := make(chan error, numWorkers)
	
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for token := range jobs {
				// Perform validation (placeholder for actual validation logic)
				if err := w.validateToken(token); err != nil {
					errors <- fmt.Errorf("token %s validation failed: %v", token.TokenID, err)
					return
				}
			}
		}()
	}
	
	// Submit jobs
	for _, token := range tokens {
		jobs <- token
	}
	close(jobs)
	
	// Wait for completion
	wg.Wait()
	close(errors)
	
	// Check for errors
	for err := range errors {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// validateToken performs individual token validation
func (w *Wallet) validateToken(token Token) error {
	// Placeholder for actual validation logic
	// This would include epoch checks, double-spend prevention, etc.
	return nil
}

// calculateTokensNeeded determines which tokens to use for the required amount
func calculateTokensNeeded(availableTokens []Token, requiredAmount float64) []Token {
	var totalValue float64
	var selectedTokens []Token
	
	// Simple greedy algorithm - select tokens until we have enough
	for _, token := range availableTokens {
		if totalValue >= requiredAmount {
			break
		}
		selectedTokens = append(selectedTokens, token)
		totalValue += token.TokenValue
	}
	
	if totalValue < requiredAmount {
		return nil // Insufficient tokens
	}
	
	return selectedTokens
}

// GetTokensForOptimizedTransfer is the main entry point for optimized token fetching
func (w *Wallet) GetTokensForOptimizedTransfer(did string, amount float64, txnMode int) ([]Token, error) {
	w.l.Lock()
	defer w.l.Unlock()
	
	startTime := time.Now()
	
	// Step 1: Batch fetch all free tokens
	var allFreeTokens []Token
	var err error
	
	if txnMode == 0 { // RBT Transfer mode
		err = w.s.Read(TokenStorage, &allFreeTokens, "did=? AND (token_status=? OR token_status=?)", did, TokenIsFree, TokenIsPinnedAsService)
	} else { // Pinning service mode
		err = w.s.Read(TokenStorage, &allFreeTokens, "did=? AND token_status=?", did, TokenIsFree)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to fetch free tokens: %v", err)
	}
	
	w.log.Info("Fetched free tokens for optimization", 
		"count", len(allFreeTokens), 
		"fetch_time", time.Since(startTime))
	
	if len(allFreeTokens) == 0 {
		return []Token{}, nil
	}
	
	// Step 2: Calculate how many tokens we need
	tokensNeeded := calculateTokensNeeded(allFreeTokens, amount)
	if len(tokensNeeded) == 0 {
		return nil, fmt.Errorf("insufficient free tokens: have %d tokens, need %.2f value", 
			len(allFreeTokens), amount)
	}
	
	w.log.Info("Tokens needed for transaction", 
		"required_value", amount,
		"token_count", len(tokensNeeded))
	
	// Step 3: Lock tokens sequentially (since we're already under wallet lock)
	lockStart := time.Now()
	for i := range tokensNeeded {
		tokensNeeded[i].TokenStatus = TokenIsLocked
		err := w.s.Update(TokenStorage, &tokensNeeded[i], "did=? AND token_id=?", did, tokensNeeded[i].TokenID)
		if err != nil {
			// Rollback previously locked tokens
			for j := 0; j < i; j++ {
				tokensNeeded[j].TokenStatus = TokenIsFree
				w.s.Update(TokenStorage, &tokensNeeded[j], "did=? AND token_id=?", did, tokensNeeded[j].TokenID)
			}
			return nil, fmt.Errorf("failed to lock token %s: %v", tokensNeeded[i].TokenID, err)
		}
	}
	
	w.log.Info("Locked tokens", 
		"count", len(tokensNeeded),
		"lock_time", time.Since(lockStart))
	
	w.log.Info("Total optimized token fetch operation", 
		"token_count", len(tokensNeeded),
		"total_time", time.Since(startTime))
	
	return tokensNeeded, nil
}