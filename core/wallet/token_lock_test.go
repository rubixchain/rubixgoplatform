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
