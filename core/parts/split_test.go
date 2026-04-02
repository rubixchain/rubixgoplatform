package parts

import (
	"testing"
)

// TestPlanTokenSplit_LevelDerivation verifies that planTokenSplit correctly derives
// the denom-tree level from the token ID rather than reading RbtIDElements.Level
// (the token-mapping level, e.g. 10001), which would cause the level guard to
// always trigger for real tokens.
func TestPlanTokenSplit_LevelDerivation(t *testing.T) {
	tests := []struct {
		name        string
		tokenID     TokenID
		needed      float64
		expectError bool
		description string
	}{
		{
			// Whole token "10001_5": denom-tree level 0, value = 1.0 RBT
			// Splitting to transfer 0.5 RBT: childLevel=1, childValue=0.5,
			// splitFactor=2, childrenNeeded=1, remainder=0
			// Should succeed with one SplitOp containing 1 child to transfer, 1 to keep.
			name:        "whole token 10001_5 split for 0.5 succeeds",
			tokenID:     "10001_5",
			needed:      0.5,
			expectError: false,
			description: "denom-tree level 0, needs 0.5 RBT; child level 1 has value 0.5",
		},
		{
			// Whole token "10001_5": denom-tree level 0, value = 1.0 RBT
			// Splitting to transfer 0.1 RBT: childLevel=1, childValue=0.5,
			// childrenNeeded=0, remainder=0.1, so we recurse into a child.
			// Should succeed producing multiple SplitOps.
			name:        "whole token 10001_5 split for 0.1 succeeds",
			tokenID:     "10001_5",
			needed:      0.1,
			expectError: false,
			description: "denom-tree level 0, needs 0.1 RBT; requires multi-level split",
		},
		{
			// Part token "10001_5_1": denom-tree level 1, value = 0.5 RBT
			// Splitting for 0.1: childLevel=2, childValue=0.1, splitFactor=5,
			// childrenNeeded=1, remainder=0
			// Should succeed.
			name:        "part token 10001_5_1 split for 0.1 succeeds",
			tokenID:     "10001_5_1",
			needed:      0.1,
			expectError: false,
			description: "denom-tree level 1, value=0.5; splitting 0.1 into level-2 children",
		},
		{
			// Leaf-level token "10001_5_333": denom-tree level 6 (max depth).
			// GetMaxDenomTreeLevel()-1 = 6, so currentLevel(6) >= 6 => error.
			// This tests that the guard still fires correctly for leaf nodes.
			name:        "leaf level token cannot be split further",
			tokenID:     "10001_5_333",
			needed:      0.001,
			expectError: true,
			description: "denom-tree level 6 is the leaf — cannot split further",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ops, err := planTokenSplit(tc.tokenID, tc.needed, nil)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for %s (%s) but got ops: %+v", tc.tokenID, tc.description, ops)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %s (%s): %v", tc.tokenID, tc.description, err)
			}
			if len(ops) == 0 {
				t.Errorf("expected at least one SplitOp for %s, got empty slice", tc.tokenID)
			}
		})
	}
}
