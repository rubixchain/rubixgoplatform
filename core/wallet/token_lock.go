package wallet

import (
	"context"
	"errors"
	"fmt"
	"math"
	stdmath "math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const (
	lockTokensForSplitRetryBudget = 3 * time.Second
	lockTokensForSplitMaxRetries  = 5
)

// ── Pure selection helper ──────────────────────────────────────────────

// selectTokensForAmount picks the minimum set of tokens to satisfy `amount`
// from the given candidate slice. Does NOT modify the slice. Pure function:
// no DB access, no Wallet receiver, no context — unit-testable in isolation.
//
// Selection policy (priority order):
//  1. Exact match: single token whose value == amount (within FloatPrecision tolerance)
//  2. Whole-number optimisation: if amount is a whole number, greedily pick 1.0-value tokens first
//  3. Exact combination: accumulate largest-first keeping sum <= amount; if result == amount, return it
//  4. Greedy fallback: accumulate smallest-first until sum >= amount
//     (prefers the smallest token >= remaining amount to minimise overshoot / split size)
//
// O(N log N) due to two sort passes — no combinatorial search.
func selectTokensForAmount(candidates []models.Token, amount float64) ([]models.Token, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("selectTokensForAmount: no candidate tokens available")
	}

	precAmount := rubixmath.FloatPrecision(amount)

	// Calculate total balance upfront.
	var totalBalance float64
	for _, tok := range candidates {
		totalBalance = rubixmath.AddFloat(totalBalance, tok.TokenValue)
	}
	if rubixmath.FloatPrecision(totalBalance) < precAmount {
		return nil, fmt.Errorf("selectTokensForAmount: insufficient balance: have %v, need %v", totalBalance, amount)
	}

	// Build a sorted-ASC copy (used by Pass 1 and Pass 4).
	ascSorted := make([]models.Token, len(candidates))
	copy(ascSorted, candidates)
	sort.Slice(ascSorted, func(i, j int) bool {
		return ascSorted[i].TokenValue < ascSorted[j].TokenValue
	})

	// ── Pass 1: Exact single-token match ──────────────────────────────
	for _, tok := range ascSorted {
		if rubixmath.FloatPrecision(tok.TokenValue) == precAmount {
			return []models.Token{tok}, nil
		}
	}

	// ── Pass 2: Whole-number optimisation ─────────────────────────────
	// If amount is a whole number, prefer collecting 1.0-value tokens first.
	isWholeNumber := precAmount == rubixmath.FloatPrecision(stdmath.Floor(amount))
	if isWholeNumber && amount >= 1.0 {
		var oneTokens []models.Token
		for _, tok := range ascSorted {
			if rubixmath.FloatPrecision(tok.TokenValue) == rubixmath.FloatPrecision(1.0) {
				oneTokens = append(oneTokens, tok)
			}
		}
		needed := int(stdmath.Round(amount)) // e.g. 2.0 → 2
		if len(oneTokens) >= needed {
			return oneTokens[:needed], nil
		}
		// Have some 1.0 tokens but not enough — use them and recurse for the remainder.
		if len(oneTokens) > 0 {
			usedValue := float64(len(oneTokens)) // each is 1.0
			remaining := rubixmath.FloatPrecision(amount - usedValue)

			// Collect non-1.0 candidates for the remaining amount.
			oneUsed := make(map[string]bool, len(oneTokens))
			for _, t := range oneTokens {
				oneUsed[t.TokenID] = true
			}
			var rest []models.Token
			for _, tok := range ascSorted {
				if !oneUsed[tok.TokenID] {
					rest = append(rest, tok)
				}
			}
			extra, err := selectTokensForAmount(rest, remaining)
			if err == nil {
				return append(oneTokens, extra...), nil
			}
			// Fall through to passes 3/4 using all candidates.
		}
	}

	// ── Pass 3: Exact combination (largest-first, sum <= amount) ──────
	// Walk tokens DESC, only adding a token when it keeps the running sum ≤ amount.
	// If the final accumulated sum equals amount exactly, we found an exact combination.
	descSorted := make([]models.Token, len(ascSorted))
	copy(descSorted, ascSorted)
	sort.Slice(descSorted, func(i, j int) bool {
		return descSorted[i].TokenValue > descSorted[j].TokenValue
	})

	var exactSelected []models.Token
	var exactSum float64
	for _, tok := range descSorted {
		precVal := rubixmath.FloatPrecision(tok.TokenValue)
		if rubixmath.FloatPrecision(exactSum+precVal) <= precAmount {
			exactSelected = append(exactSelected, tok)
			exactSum = rubixmath.AddFloat(exactSum, precVal)
			if exactSum == precAmount {
				return exactSelected, nil
			}
		}
	}
	// exactSum < precAmount here — no exact subset found. Fall through to greedy.

	// ── Pass 4: Greedy smallest-first fallback ────────────────────────
	// Accumulate tokens ASC until sum >= amount.
	// Before each addition, if the current token alone covers the remaining
	// amount, take it and stop (minimises overshoot / split size — ASC order
	// guarantees this is the smallest such token).
	var selected []models.Token
	var accumulated float64

	for _, tok := range ascSorted {
		precVal := rubixmath.FloatPrecision(tok.TokenValue)
		remaining := rubixmath.FloatPrecision(precAmount - accumulated)

		// If this token alone covers the remaining amount, take it and stop.
		if precVal >= remaining {
			selected = append(selected, tok)
			accumulated = rubixmath.AddFloat(accumulated, precVal)
			break
		}

		selected = append(selected, tok)
		accumulated = rubixmath.AddFloat(accumulated, precVal)

		if accumulated >= precAmount {
			break
		}
	}

	if rubixmath.FloatPrecision(accumulated) < precAmount {
		return nil, fmt.Errorf("selectTokensForAmount: insufficient tokens: have %v, need %v", accumulated, amount)
	}

	return selected, nil
}

// ── Tx-accepting query helpers ─────────────────────────────────────────
// These run SELECT ... FOR UPDATE within a caller-managed transaction.
// They do NOT update token_status or commit — the caller is responsible.

// QueryAndLockFTs selects and locks FT tokens within an existing transaction.
// Joins with the fts table to filter by ft_name and creator_did.
// Returns exactly `count` tokens or an error.
func (w *Wallet) QueryAndLockFTs(ctx context.Context, tx pgx.Tx, ownerDID string, ftName string, creatorDID string, count int) ([]models.Token, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
			t.did, t.transaction_id, t.token_state_hash, t.token_type,
			t.latest_position, t.latest_role, t.created_at, t.updated_at
		FROM tokens t
		INNER JOIN ft_tokens ft ON ft.token_id = t.token_id
		INNER JOIN fts f ON f.id = ft.ft_id
		WHERE t.token_type = (SELECT id FROM token_type WHERE name = $1)
			AND t.did = $2
			AND t.token_status = $3
			AND f.ft_name = $4
			AND f.creator_did = $5
		ORDER BY t.token_id
		LIMIT $6
		FOR UPDATE OF t
	`, constants.TokenType_FT, ownerDID, constants.TokenStatus_Free, ftName, creatorDID, count)
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockFTs: query: %w", err)
	}

	tokens, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Token])
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockFTs: collect: %w", err)
	}

	if len(tokens) < count {
		return nil, fmt.Errorf("QueryAndLockFTs: insufficient FTs: have %d, need %d for ft_name=%s creator=%s",
			len(tokens), count, ftName, creatorDID)
	}

	return tokens, nil
}

// QueryAndLockByIDs selects and locks tokens by their IDs within an existing transaction.
// tokenIDs must be pre-sorted by the caller for deadlock prevention.
// Returns all matched tokens or an error if any are missing/not-free/not-owned.
func (w *Wallet) QueryAndLockByIDs(ctx context.Context, tx pgx.Tx, ownerDID string, tokenIDs []string, tokenTypeName string) ([]models.Token, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
		       t.did, t.transaction_id, t.token_state_hash, t.token_type,
		       t.latest_position, t.latest_role, t.created_at, t.updated_at
		FROM tokens t
		WHERE t.token_id = ANY($1::text[])
		  AND t.did = $2
		  AND t.token_type = (SELECT id FROM token_type WHERE name = $3)
		  AND t.token_status = $4
		ORDER BY t.token_id
		FOR UPDATE OF t
	`, tokenIDs, ownerDID, tokenTypeName, constants.TokenStatus_Free)
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): query: %w", tokenTypeName, err)
	}

	locked, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Token])
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): collect: %w", tokenTypeName, err)
	}

	if len(locked) != len(tokenIDs) {
		foundSet := make(map[string]bool, len(locked))
		for _, tok := range locked {
			foundSet[tok.TokenID] = true
		}
		var missing []string
		for _, id := range tokenIDs {
			if !foundSet[id] {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("QueryAndLockByIDs(%s): tokens not found, not owned by %s, or already locked: %v",
			tokenTypeName, ownerDID, missing)
	}

	return locked, nil
}

// QueryAndLockForExecution locks NFT or SmartContract tokens for execution.
// Accepts tokens in Deployed or Executed status (after previous deployment/execution).
// For NFTs, tokens in Free status are also accepted to support execution after
// ownership transfer (where the receiver's NFT lands in Free status).
// tokenIDs must be pre-sorted by the caller for deadlock prevention.
// checkOwnership controls whether tokens.did must match ownerDID:
//   - true  for NFT with TransferNFTOwnership=true: only the current owner may execute
//   - false for SC (any subscriber may execute) and NFT with TransferNFTOwnership=false
//
// Returns all matched tokens or an error if any are missing or not in executable status.
func (w *Wallet) QueryAndLockForExecution(ctx context.Context, tx pgx.Tx, ownerDID string, tokenIDs []string, tokenTypeName string, checkOwnership bool) ([]models.Token, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	var (
		rows pgx.Rows
		err  error
	)

	if checkOwnership {
		rows, err = tx.Query(ctx, `
			SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
			       t.did, t.transaction_id, t.token_state_hash, t.token_type,
			       t.latest_position, t.latest_role, t.created_at, t.updated_at
			FROM tokens t
			WHERE t.token_id = ANY($1::text[])
			  AND t.did = $2
			  AND t.token_type = (SELECT id FROM token_type WHERE name = $3)
			  AND (t.token_status = $4 OR t.token_status = $5
			       OR ($3 = 'nft' AND t.token_status = $6))
			ORDER BY t.token_id
			FOR UPDATE OF t
		`, tokenIDs, ownerDID, tokenTypeName, constants.TokenStatus_Deployed, constants.TokenStatus_Executed, constants.TokenStatus_Free)
	} else {
		// SC execution or NFT without ownership transfer: any subscriber can execute.
		rows, err = tx.Query(ctx, `
			SELECT t.token_id, t.parent_token_id, t.token_value, t.token_status,
			       t.did, t.transaction_id, t.token_state_hash, t.token_type,
			       t.latest_position, t.latest_role, t.created_at, t.updated_at
			FROM tokens t
			WHERE t.token_id = ANY($1::text[])
			  AND t.token_type = (SELECT id FROM token_type WHERE name = $2)
			  AND (t.token_status = $3 OR t.token_status = $4
			       OR ($2 = 'nft' AND t.token_status = $5))
			ORDER BY t.token_id
			FOR UPDATE OF t
		`, tokenIDs, tokenTypeName, constants.TokenStatus_Deployed, constants.TokenStatus_Executed, constants.TokenStatus_Free)
	}
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockForExecution(%s): query: %w", tokenTypeName, err)
	}

	locked, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Token])
	if err != nil {
		return nil, fmt.Errorf("QueryAndLockForExecution(%s): collect: %w", tokenTypeName, err)
	}

	if len(locked) != len(tokenIDs) {
		foundSet := make(map[string]bool, len(locked))
		for _, tok := range locked {
			foundSet[tok.TokenID] = true
		}
		var missing []string
		for _, id := range tokenIDs {
			if !foundSet[id] {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("QueryAndLockForExecution(%s): tokens not found or not in executable status (Deployed/Executed, or Free for NFT): %v",
			tokenTypeName, missing)
	}

	return locked, nil
}

// ── Self-contained wrappers ────────────────────────────────────────────
// These open their own transaction, lock tokens, update status, and commit.
// Use these when locking a single asset type independently.

// lockTokensByIDs is the internal batch lock helper. Opens a DB transaction,
// locks all tokens matching the given IDs + type + owner + free status,
// validates all were found, updates status to Locked, and commits.
func (w *Wallet) lockTokensByIDs(ctx context.Context, ownerDID string, tokenIDs []string, tokenTypeName string, label string) ([]models.Token, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	sort.Strings(tokenIDs)

	tx, err := w.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", label, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("%s: set lock_timeout: %w", label, err)
	}

	locked, err := w.QueryAndLockByIDs(ctx, tx, ownerDID, tokenIDs, tokenTypeName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
		constants.TokenStatus_Locked, time.Now(), tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: update status: %w", label, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", label, err)
	}

	return locked, nil
}

// LockFTTokens selects and locks FT tokens matching ft_name and creator_did.
// Self-contained: opens its own transaction.
func (w *Wallet) LockFTTokens(ctx context.Context, ownerDID string, ftName string, creatorDID string, count int) ([]models.Token, error) {
	tx, err := w.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("LockFTTokens: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("LockFTTokens: set lock_timeout: %w", err)
	}

	selected, err := w.QueryAndLockFTs(ctx, tx, ownerDID, ftName, creatorDID, count)
	if err != nil {
		return nil, err
	}

	tokenIDs := make([]string, len(selected))
	for i, tok := range selected {
		tokenIDs[i] = tok.TokenID
	}

	_, err = tx.Exec(ctx,
		`UPDATE tokens SET token_status = $1, updated_at = $2 WHERE token_id = ANY($3::text[])`,
		constants.TokenStatus_Locked, time.Now(), tokenIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("LockFTTokens: update status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("LockFTTokens: commit: %w", err)
	}

	return selected, nil
}

func SelectTokensFromTokenDenomForLocking(
	inputTokenDenom map[types.DenomValue]types.DenomCount,
	transferAmount float64,
) (denomArrForTokenSelection map[types.DenomValue]types.DenomCount, err error) {
	denomArrForTokenSelection = make(map[types.DenomValue]types.DenomCount)

	remaining := rubixmath.FloatPrecision(transferAmount)

	var denomsArr []types.DenomValue = make([]types.DenomValue, 0)
	for denom := range inputTokenDenom {
		denomsArr = append(denomsArr, denom)
	}
	sort.Slice(denomsArr, func(i, j int) bool {
		return denomsArr[i] > denomsArr[j]
	})

	for _, denomValue := range denomsArr {
		if remaining <= 0 {
			break
		}

		denomCount := inputTokenDenom[denomValue]

		maxByTarget := int(math.Ceil(rubixmath.ScaledFloatDiv(remaining, denomValue)))

		canTake := rubixmath.Min(int(denomCount), maxByTarget)

		if canTake > 0 {
			amount := rubixmath.FloatPrecision(float64(canTake) * denomValue)
			denomArrForTokenSelection[denomValue] = int64(canTake)
			remaining = rubixmath.FloatPrecision(remaining - amount)
		}
	}

	return denomArrForTokenSelection, nil
}


// LockTokensForSplit selects and locks the minimum set of free RBT tokens needed
// for the given amount, using a three-phase approach:
//
//  1. SELECT all free RBT tokens FOR UPDATE SKIP LOCKED (atomic row-level lock).
//  2. selectTokensForAmount chooses the minimum subset in Go (denomination-aware).
//  3. UPDATE token_status to Locked + commit; non-selected rows release locks.
//
// Returns only the selected (now-locked) tokens; callers must eventually release or consume them.
func (w *Wallet) LockTokensForSplit(ctx context.Context, ownerDID string, amount float64, referenceID string) ([]models.Token, error) {
	w.log.Info("LockTokensForSplit: locking tokens for split", "ownerDID", ownerDID, "amount", amount)
	retryCtx, cancel := context.WithTimeout(ctx, lockTokensForSplitRetryBudget)
	defer cancel()
	deadline := time.Now().Add(lockTokensForSplitRetryBudget)

	var lastErr error
	for retry := 0; retry <= lockTokensForSplitMaxRetries; retry++ {
		selected, err := w.lockTokensForSplitOnce(retryCtx, ownerDID, amount, referenceID)
		if err == nil {
			return selected, nil
		}
		lastErr = err

		if !shouldRetryLockTokensForSplit(err) {
			return nil, err
		}
		if retry == lockTokensForSplitMaxRetries {
			break
		}

		sleepFor := lockTokensForSplitJitter(time.Until(deadline), lockTokensForSplitMaxRetries-retry)
		if sleepFor <= 0 {
			break
		}

		w.log.Warn("LockTokensForSplit: contention detected, retrying",
			"retry", retry+1,
			"maxRetries", lockTokensForSplitMaxRetries,
			"sleepFor", sleepFor,
			"err", err,
		)

		select {
		case <-time.After(sleepFor):
		case <-retryCtx.Done():
			return nil, fmt.Errorf("LockTokensForSplit: retry budget exhausted after %s: %w", lockTokensForSplitRetryBudget, lastErr)
		}
	}

	if errors.Is(retryCtx.Err(), context.DeadlineExceeded) && lastErr != nil {
		return nil, fmt.Errorf("LockTokensForSplit: retry budget exhausted after %s: %w", lockTokensForSplitRetryBudget, lastErr)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("LockTokensForSplit: retries exhausted after %d retries: %w", lockTokensForSplitMaxRetries, lastErr)
	}

	return nil, fmt.Errorf("LockTokensForSplit: retries exhausted without a concrete error")
}

func (w *Wallet) lockTokensForSplitOnce(
	ctx context.Context,
	ownerDID string,
	amount float64,
	referenceID string,
) ([]models.Token, error) {
	tokenDenom, err := w.GetTokenDenomArray(ownerDID)
	if err != nil {
		return nil, fmt.Errorf("lockTokensForSplitOnce: failed to get token denom for DID: %v, err: %v", ownerDID, err)
	}

	tx, err := w.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '1500ms'"); err != nil {
		return nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	precAmount := rubixmath.FloatPrecision(amount)

	denomArrayForTokenSelection, err := SelectTokensFromTokenDenomForLocking(tokenDenom, precAmount)
	if err != nil {
		return nil, err
	}

	var tokensToLockForTransaction []models.Token

	for denomValue, denomCount := range denomArrayForTokenSelection {
		rows, err := tx.Query(ctx, `
			SELECT token_id, parent_token_id, token_value, token_status, did,
			       transaction_id, token_state_hash, token_type,
			       latest_position, latest_role, created_at, updated_at
			FROM tokens
			WHERE did = $1
			  AND token_type = (SELECT id FROM token_type WHERE name = $2)
			  AND token_status = $3
			  AND token_value = $4
			ORDER BY token_value DESC, token_id ASC
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		`, ownerDID, constants.TokenType_RBT, constants.TokenStatus_Free, denomValue, denomCount)
		if err != nil {
			return nil, fmt.Errorf("scan query: %w", err)
		}

		denomTokens, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Token])
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan collect: %w", err)
		}

		tokensToLockForTransaction = append(tokensToLockForTransaction, denomTokens...)
	}
	
	return w.lockSelectedTokens(ctx, tx, tokensToLockForTransaction, referenceID)
}

func (w *Wallet) AddLockReferenceForToken(ctx context.Context,
	tx pgx.Tx,
	selected []models.Token,
	referenceID string) ([]models.Token, error) {
	return w.lockSelectedTokens(ctx, tx, selected, referenceID)
}

func (w *Wallet) lockSelectedTokens(
	ctx context.Context,
	tx pgx.Tx,
	selected []models.Token,
	referenceID string,
) ([]models.Token, error) {

	if len(selected) == 0 {
		return nil, fmt.Errorf("lockSelectedTokens: no tokens provided")
	}

	// Extract token IDs
	selectedIDs := make([]string, len(selected))
	for i, tok := range selected {
		selectedIDs[i] = tok.TokenID
	}

	if referenceID == "" {
		return nil, fmt.Errorf("lockSelectedTokens: referenceID is required")
	}

	_, err := tx.Exec(ctx, `
		UPDATE tokens
		SET token_status = $1,
		    updated_at = NOW(),
		    lock_reference_id = $3
		WHERE token_id = ANY($2::text[])
		  AND token_status = $4
	`,
		constants.TokenStatus_Locked,
		selectedIDs,
		referenceID,
		constants.TokenStatus_Free,
	)
	if err != nil {
		return nil, fmt.Errorf("lockSelectedTokens: update failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("lockSelectedTokens: commit failed: %w", err)
	}

	return selected, nil
}

func shouldRetryLockTokensForSplit(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "selectTokensForAmount: no candidate tokens available") ||
		strings.Contains(msg, "selectTokensForAmount: insufficient balance") ||
		strings.Contains(msg, "LockTokensForSplit: candidate query:") ||
		strings.Contains(msg, "lock timeout") ||
		strings.Contains(msg, "deadlock detected") ||
		strings.Contains(msg, "could not serialize access")
}

func lockTokensForSplitJitter(remaining time.Duration, retriesLeft int) time.Duration {
	if remaining <= 0 || retriesLeft <= 0 {
		return 0
	}

	maxSleep := remaining / time.Duration(retriesLeft)
	if maxSleep <= 0 {
		return 0
	}

	return time.Duration(rand.Int63n(int64(maxSleep) + 1))
}

// LockNFTTokens locks NFT tokens by IDs. Self-contained.
func (w *Wallet) LockNFTTokens(ctx context.Context, ownerDID string, tokenIDs []string) ([]models.Token, error) {
	return w.lockTokensByIDs(ctx, ownerDID, tokenIDs, constants.TokenType_NFT, "LockNFTTokens")
}

// LockSmartContractTokens locks SC tokens by IDs. Self-contained.
func (w *Wallet) LockSmartContractTokens(ctx context.Context, ownerDID string, tokenIDs []string) ([]models.Token, error) {
	return w.lockTokensByIDs(ctx, ownerDID, tokenIDs, constants.TokenType_SmartContract, "LockSmartContractTokens")
}

// LockNFTToken locks a single NFT token. Convenience wrapper.
func (w *Wallet) LockNFTToken(ctx context.Context, ownerDID string, tokenID string) (models.Token, error) {
	tokens, err := w.LockNFTTokens(ctx, ownerDID, []string{tokenID})
	if err != nil {
		return models.Token{}, err
	}
	return tokens[0], nil
}

// LockSmartContractToken locks a single SC token. Convenience wrapper.
func (w *Wallet) LockSmartContractToken(ctx context.Context, ownerDID string, tokenID string) (models.Token, error) {
	tokens, err := w.LockSmartContractTokens(ctx, ownerDID, []string{tokenID})
	if err != nil {
		return models.Token{}, err
	}
	return tokens[0], nil
}

// UnlockLockedTokens releases specific locked tokens for a DID back to Free status.
// Called during transaction abort to return locked tokens to their Free state.
func (w *Wallet) UnlockLockedTokens(did string, tokens []string, referenceID string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := w.db.Pool().Exec(w.Ctx,
		`UPDATE tokens SET token_status=$1, updated_at=$2, lock_reference_id=NULL
		 WHERE did=$3 AND token_id = ANY($4::text[]) AND token_status=$5 AND lock_reference_id=$6`,
		constants.TokenStatus_Free, time.Now(), did, tokens, constants.TokenStatus_Locked, referenceID,
	)
	return err
}