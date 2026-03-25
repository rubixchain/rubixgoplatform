package util

import (
	"fmt"
	"math"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
)

func TestGetTokenValueAtLevel(t *testing.T) {
	// compute max depth using the same logic as computeTreeLevelRanges
	// so the test self-adjusts if constants.MaxDecimalPlaces ever changes
	scaledValue := int(math.Pow10(constants.MaxSupportedDecimalPlaces))
	maxDepth := 0
	current := scaledValue
	for current > 1 {
		current /= GetNumberOfChildren(maxDepth)
		maxDepth++
	}

	// compute expected value at each level using the same
	// scaled integer approach as GetTokenValueAtLevel itself
	expectedValueAtLevel := func(level int) float64 {
		scaled := scaledValue
		for l := 0; l < level; l++ {
			scaled /= GetNumberOfChildren(l)
		}
		return float64(scaled) / float64(scaledValue)
	}

	// build valid level test cases dynamically
	tests := []struct {
		name        string
		level       int
		expected    float64
		expectError bool
	}{}

	for l := 0; l <= maxDepth; l++ {
		tests = append(tests, struct {
			name        string
			level       int
			expected    float64
			expectError bool
		}{
			name:        fmt.Sprintf("L%d value %.*f", l, constants.MaxSupportedDecimalPlaces, expectedValueAtLevel(l)),
			level:       l,
			expected:    expectedValueAtLevel(l),
			expectError: false,
		})
	}

	// append error cases
	tests = append(tests,
		struct {
			name        string
			level       int
			expected    float64
			expectError bool
		}{"negative level", -1, 0, true},
		struct {
			name        string
			level       int
			expected    float64
			expectError bool
		}{"level beyond max depth", maxDepth + 1, 0, true},
	)

	const epsilon = 1e-9

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := LevelToDenom(tc.level)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got %v", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := result - tc.expected; diff > epsilon || diff < -epsilon {
				t.Errorf("got %v, want %v (diff %e)", result, tc.expected, diff)
			}
		})
	}
}
