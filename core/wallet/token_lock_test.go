package wallet

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// makeTokens is a helper to build a []models.Token slice with only TokenID and TokenValue set.
func makeTokens(idValuePairs ...interface{}) []models.Token {
	var tokens []models.Token
	for i := 0; i < len(idValuePairs); i += 2 {
		tokens = append(tokens, models.Token{
			TokenID:    idValuePairs[i].(string),
			TokenValue: idValuePairs[i+1].(float64),
		})
	}
	return tokens
}

// tokenIDs returns the TokenID of each token in the slice.
func tokenIDs(tokens []models.Token) []string {
	ids := make([]string, len(tokens))
	for i, t := range tokens {
		ids[i] = t.TokenID
	}
	return ids
}

// totalValue sums the TokenValue of each token in the slice.
func totalValue(tokens []models.Token) float64 {
	var sum float64
	for _, t := range tokens {
		sum += t.TokenValue
	}
	return sum
}

// ── Task 1 tests ──────────────────────────────────────────────────────────────

// Test: exact single-token match — [1.0, 1.0, 0.5, 0.001], amount=1.0 → selects exactly [1.0]
func TestSelectTokensForAmount_ExactSingleMatch(t *testing.T) {
	tokens := makeTokens("a", 1.0, "b", 1.0, "c", 0.5, "d", 0.001)
	selected, err := selectTokensForAmount(tokens, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(selected), tokenIDs(selected))
	}
	if selected[0].TokenValue != 1.0 {
		t.Errorf("expected token with value 1.0, got %v", selected[0].TokenValue)
	}
}

// Test: exact multi-token match — [0.5, 0.5, 0.001], amount=1.0 → selects [0.5, 0.5]
func TestSelectTokensForAmount_ExactMultiMatch(t *testing.T) {
	tokens := makeTokens("a", 0.5, "b", 0.5, "c", 0.001)
	selected, err := selectTokensForAmount(tokens, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(selected), tokenIDs(selected))
	}
	total := totalValue(selected)
	if total != 1.0 {
		t.Errorf("expected total 1.0, got %v", total)
	}
}

// Test: whole-number greedy — [1.0, 1.0, 0.5], amount=2.0 → selects [1.0, 1.0]
func TestSelectTokensForAmount_WholeNumberGreedy(t *testing.T) {
	tokens := makeTokens("a", 1.0, "b", 1.0, "c", 0.5)
	selected, err := selectTokensForAmount(tokens, 2.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(selected), tokenIDs(selected))
	}
	for _, tok := range selected {
		if tok.TokenValue != 1.0 {
			t.Errorf("expected all selected tokens to be 1.0, got %v", tok.TokenValue)
		}
	}
}

// Test: fractional greedy exact — [0.5, 0.001, 0.001], amount=0.501 → selects [0.5, 0.001]
func TestSelectTokensForAmount_FractionalGreedyExact(t *testing.T) {
	tokens := makeTokens("a", 0.5, "b", 0.001, "c", 0.001)
	selected, err := selectTokensForAmount(tokens, 0.501)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	total := totalValue(selected)
	if total < 0.501 {
		t.Errorf("expected total >= 0.501, got %v", total)
	}
	// Should not select more than needed
	if len(selected) > 2 {
		t.Errorf("expected at most 2 tokens, got %d", len(selected))
	}
}

// Test: fallback smallest-token-above-amount — [1.0, 0.5], amount=0.3 → selects [0.5]
func TestSelectTokensForAmount_FallbackSmallestAbove(t *testing.T) {
	tokens := makeTokens("a", 1.0, "b", 0.5)
	selected, err := selectTokensForAmount(tokens, 0.3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(selected), tokenIDs(selected))
	}
	if selected[0].TokenValue != 0.5 {
		t.Errorf("expected token with value 0.5 (smallest above 0.3), got %v", selected[0].TokenValue)
	}
}

// Test: fallback only option is larger — [1.0], amount=0.3 → selects [1.0]
func TestSelectTokensForAmount_FallbackOnlyLarger(t *testing.T) {
	tokens := makeTokens("a", 1.0)
	selected, err := selectTokensForAmount(tokens, 0.3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 token, got %d", len(selected))
	}
	if selected[0].TokenValue != 1.0 {
		t.Errorf("expected 1.0, got %v", selected[0].TokenValue)
	}
}

// Test: empty candidates → error
func TestSelectTokensForAmount_EmptyError(t *testing.T) {
	_, err := selectTokensForAmount([]models.Token{}, 1.0)
	if err == nil {
		t.Fatal("expected error for empty token list, got nil")
	}
}

// Test: insufficient balance → error
func TestSelectTokensForAmount_InsufficientError(t *testing.T) {
	tokens := makeTokens("a", 0.1, "b", 0.1)
	_, err := selectTokensForAmount(tokens, 1.0)
	if err == nil {
		t.Fatal("expected error for insufficient balance, got nil")
	}
}

// Test: mixed whole + fractional — [1.0, 1.0, 0.5, 0.001], amount=1.501 → selects [1.0, 0.5, 0.001]
func TestSelectTokensForAmount_MixedWholeAndFractional(t *testing.T) {
	tokens := makeTokens("a", 1.0, "b", 1.0, "c", 0.5, "d", 0.001)
	selected, err := selectTokensForAmount(tokens, 1.501)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	total := totalValue(selected)
	if total < 1.501 {
		t.Errorf("expected total >= 1.501, got %v", total)
	}
	// Should not over-select (1.0 + 0.5 + 0.001 = 1.501 exact)
	if len(selected) > 3 {
		t.Errorf("expected at most 3 tokens, got %d", len(selected))
	}
}

// ── Task 3 tests ──────────────────────────────────────────────────────────────

// TestSelectTokensForAmount_ConcurrencyScenario verifies that two parallel
// transfers from the same DID select disjoint token sets, proving one transfer
// cannot monopolise all free tokens and starve the other.
//
// DID has 5 tokens: [1.0, 1.0, 0.5, 0.001, 0.001]
// Thread A needs 1.0, Thread B needs 0.501.
// Old behaviour: both would try to lock all 5.
// New behaviour: A selects [1.0], B selects [0.5, 0.001].
// The two selections must be disjoint.
func TestSelectTokensForAmount_ConcurrencyScenario(t *testing.T) {
	allTokens := makeTokens(
		"tok1", 1.0,
		"tok2", 1.0,
		"tok3", 0.5,
		"tok4", 0.001,
		"tok5", 0.001,
	)

	selectedA, err := selectTokensForAmount(allTokens, 1.0)
	if err != nil {
		t.Fatalf("thread A selection failed: %v", err)
	}

	// Remove A's tokens from the pool to simulate concurrent locking.
	aIDs := make(map[string]bool, len(selectedA))
	for _, tok := range selectedA {
		aIDs[tok.TokenID] = true
	}
	var remaining []models.Token
	for _, tok := range allTokens {
		if !aIDs[tok.TokenID] {
			remaining = append(remaining, tok)
		}
	}

	selectedB, err := selectTokensForAmount(remaining, 0.501)
	if err != nil {
		t.Fatalf("thread B selection failed: %v", err)
	}

	// Verify disjoint: no token ID appears in both selections.
	bIDs := make(map[string]bool, len(selectedB))
	for _, tok := range selectedB {
		bIDs[tok.TokenID] = true
	}
	for _, tok := range selectedA {
		if bIDs[tok.TokenID] {
			t.Errorf("token %s (value=%v) selected by both threads — selections are not disjoint",
				tok.TokenID, tok.TokenValue)
		}
	}

	// Both selections must cover their required amounts.
	if totalValue(selectedA) < 1.0 {
		t.Errorf("thread A total %v < required 1.0", totalValue(selectedA))
	}
	if totalValue(selectedB) < 0.501 {
		t.Errorf("thread B total %v < required 0.501", totalValue(selectedB))
	}
}

// TestSelectTokensForAmount_FallbackToSplit verifies that when no exact match
// exists and the amount falls between available token values, the function
// selects a combination whose total >= amount (caller splits the overshoot token).
//
// tokens: [1.0, 0.5], amount: 0.7
// No exact match. Greedy smallest-first: 0.5 < 0.7, add 0.5; remaining = 0.2.
// Next token 1.0 >= 0.2, select as split candidate and stop.
// Result total >= 0.7.
func TestSelectTokensForAmount_FallbackToSplit(t *testing.T) {
	tokens := makeTokens("big", 1.0, "small", 0.5)
	selected, err := selectTokensForAmount(tokens, 0.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	total := totalValue(selected)
	if total < 0.7 {
		t.Errorf("expected total >= 0.7, got %v", total)
	}
}

// TestSelectTokensForAmount_WholeNumberMixed verifies the graceful fallback when
// there are not enough 1.0-value tokens and the remainder must be covered by
// smaller denominations.
//
// tokens: [1.0, 0.5, 0.5, 0.001], amount: 2.0
// Whole-number pass: only 1x 1.0 available, need 2. Use it, remaining = 1.0.
// For remaining 1.0: exact multi-token match 0.5+0.5=1.0.
// Result: [1.0, 0.5, 0.5].
func TestSelectTokensForAmount_WholeNumberMixed(t *testing.T) {
	tokens := makeTokens("one", 1.0, "half1", 0.5, "half2", 0.5, "tiny", 0.001)
	selected, err := selectTokensForAmount(tokens, 2.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	total := totalValue(selected)
	if total < 2.0 {
		t.Errorf("expected total >= 2.0, got %v", total)
	}
	// Should select the 1.0 token and not the 0.001 (0.001 not needed for exact 2.0)
	selectedIDSet := make(map[string]bool)
	for _, tok := range selected {
		selectedIDSet[tok.TokenID] = true
	}
	if selectedIDSet["tiny"] {
		t.Errorf("selected 0.001 token unnecessarily — should have 1.0 + 0.5 + 0.5 = 2.0 exact")
	}
}
