package util

import (
	"fmt"
	"math"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
)

func TestParseTokenLevel(t *testing.T) {
	tests := []struct {
		name        string
		tokenID     string
		wantLevel   int
		expectError bool
	}{
		// Whole token: PartIndex=0, denom-tree level 0
		{
			name:        "whole token 10001_5 returns level 0",
			tokenID:     "10001_5",
			wantLevel:   0,
			expectError: false,
		},
		// Part token: partIndex=1 is in L1 range [1,2] -> denom-tree level 1
		{
			name:        "part token 10001_5_1 returns level 1",
			tokenID:     "10001_5_1",
			wantLevel:   1,
			expectError: false,
		},
		// partIndex=3 is in L2 range [3,12] -> denom-tree level 2
		{
			name:        "part token 10001_5_3 returns level 2",
			tokenID:     "10001_5_3",
			wantLevel:   2,
			expectError: false,
		},
		// Error cases
		{
			name:        "badtoken without underscore returns error",
			tokenID:     "badtoken",
			expectError: true,
		},
		{
			name:        "empty string returns error",
			tokenID:     "",
			expectError: true,
		},
		{
			name:        "abc_def with non-numeric elements returns error",
			tokenID:     "abc_def",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			level, err := ParseTokenLevel(tc.tokenID)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got level=%d", level)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if level != tc.wantLevel {
				t.Errorf("got level=%d, want level=%d", level, tc.wantLevel)
			}
		})
	}
}

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
