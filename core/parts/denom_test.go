package parts

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/types"
)

func TestHeirarchicalToIndexed(t *testing.T) {
	tests := []struct {
		name        string
		input       TokenID
		expected    string
		expectError bool
	}{
		{
			name:        "simple path 1",
			input:       "34_2345_1",
			expected:    "34_2345_1",
			expectError: false,
		},
		{
			name:        "simple path 2",
			input:       "34_2345_2",
			expected:    "34_2345_2",
			expectError: false,
		},
		{
			name:        "depth 2 path 1_1",
			input:       "34_2345_1_1",
			expected:    "34_2345_3",
			expectError: false,
		},
		{
			name:        "depth 2 path 1_2",
			input:       "34_2345_1_2",
			expected:    "34_2345_4",
			expectError: false,
		},
		{
			name:        "depth 2 path 1_5",
			input:       "34_2345_1_5",
			expected:    "34_2345_7",
			expectError: false,
		},
		{
			name:        "depth 3 path 1_1_1",
			input:       "34_2345_1_1_1",
			expected:    "34_2345_13",
			expectError: false,
		},
		{
			name:        "depth 3 path 1_1_2",
			input:       "34_2345_1_1_2",
			expected:    "34_2345_14",
			expectError: false,
		},
		{
			name:        "whole token",
			input:       "34_2345",
			expected:    "34_2345",
			expectError: false,
		},
		{
			name:        "invalid child at depth 0",
			input:       "34_2345_3",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid child at depth 1",
			input:       "34_2345_1_6",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HeirarchicalToIndexed(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, result: %s", result)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("HeirarchicalToIndexed(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIndexedToHierarchical(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    TokenID
		expectError bool
	}{
		{
			name:        "index 1 -> path 1",
			input:       "34_2345_1",
			expected:    "34_2345_1",
			expectError: false,
		},
		{
			name:        "index 2 -> path 2",
			input:       "34_2345_2",
			expected:    "34_2345_2",
			expectError: false,
		},
		{
			name:        "index 3 -> path 1_1",
			input:       "34_2345_3",
			expected:    "34_2345_1_1",
			expectError: false,
		},
		{
			name:        "index 4 -> path 1_2",
			input:       "34_2345_4",
			expected:    "34_2345_1_2",
			expectError: false,
		},
		{
			name:        "index 7 -> path 1_5",
			input:       "34_2345_7",
			expected:    "34_2345_1_5",
			expectError: false,
		},
		{
			name:        "index 8 -> path 2_1",
			input:       "34_2345_8",
			expected:    "34_2345_2_1",
			expectError: false,
		},
		{
			name:        "index 13 -> path 1_1_1",
			input:       "34_2345_13",
			expected:    "34_2345_1_1_1",
			expectError: false,
		},
		{
			name:        "index 14 -> path 1_1_2",
			input:       "34_2345_14",
			expected:    "34_2345_1_1_2",
			expectError: false,
		},
		{
			name:        "whole token",
			input:       "34_2345",
			expected:    "34_2345",
			expectError: false,
		},
		{
			name:        "invalid indexed number",
			input:       "34_2345_abc",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IndexedToHierarchical(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, result: %s", result)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("IndexedToHierarchical(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRoundTripConversion(t *testing.T) {
	hierarchicalPaths := []string{
		"34_2345",
		"34_2345_1",
		"34_2345_2",
		"34_2345_1_1",
		"34_2345_1_2",
		"34_2345_1_3",
		"34_2345_1_4",
		"34_2345_1_5",
		"34_2345_2_1",
		"34_2345_2_5",
		"34_2345_1_1_1",
		"34_2345_1_1_2",
		"34_2345_1_5_1",
		"34_2345_1_5_2",
		"34_2345_1_1_1_1",
		"34_2345_1_1_1_2",
		"34_2345_1_1_1_3",
		"34_2345_1_1_1_4",
		"34_2345_1_1_1_5",
	}

	for _, path := range hierarchicalPaths {
		t.Run("roundtrip_"+path, func(t *testing.T) {
			// Hierarchical -> Indexed
			indexed, err := HeirarchicalToIndexed(TokenID(path))
			if err != nil {
				t.Fatalf("HeirarchicalToIndexed failed: %v", err)
			}

			// Indexed -> Hierarchical
			backToHierarchical, err := IndexedToHierarchical(indexed)
			if err != nil {
				t.Fatalf("IndexedToHierarchical failed: %v", err)
			}

			if string(backToHierarchical) != path {
				t.Errorf("round trip failed: %s -> %s -> %s", path, indexed, backToHierarchical)
			}
		})
	}
}

func TestGetChildrenIndex(t *testing.T) {
	tests := []struct {
		name        string
		parent      TokenID
		expected    types.ChildrenRange
		expectError bool
	}{
		{
			// x=0, Lx=0 (even→2 children), Min(L1)=1
			// levelIndex = 0-0 = 0
			// firstChild = (0 * 2) + 1 = 1, lastChild = 1 + 1 = 2
			name:        "whole token → children 1 to 2",
			parent:      "100_76",
			expected:    types.ChildrenRange{First: 1, Last: 2},
			expectError: false,
		},
		{
			// x=19, Lx=3 (odd→5 children), Min(L4)=33
			// levelIndex = 19 - 13 = 6
			// firstChild = (6 * 5) + 33 = 63, lastChild = 63 + 4 = 67
			name:        "L3 index 19 → children 63 to 67",
			parent:      "1_5_19",
			expected:    types.ChildrenRange{First: 63, Last: 67},
			expectError: false,
		},
		{
			// x=65, Lx=4 (even→2 children), Min(L5)=133
			// levelIndex = 65 - 33 = 32
			// firstChild = (32 * 2) + 133 = 197, lastChild = 197 + 1 = 198
			name:        "L4 index 65 → children 197 to 198",
			parent:      "1_5_65",
			expected:    types.ChildrenRange{First: 197, Last: 198},
			expectError: false,
		},
		{
			// x=87, Lx=4 (even→2 children), Min(L5)=133
			// levelIndex = 87 - 33 = 54
			// firstChild = (54 * 2) + 133 = 241, lastChild = 241 + 1 = 242
			name:        "L4 index 87 → children 241 to 242",
			parent:      "16_8_87",
			expected:    types.ChildrenRange{First: 241, Last: 242},
			expectError: false,
		},
		{
			// x=20, Lx=3 (odd→5 children), Min(L4)=33
			// levelIndex = 20 - 13 = 7
			// firstChild = (7 * 5) + 33 = 68, lastChild = 68 + 4 = 72
			name:        "L3 index 20 → children 68 to 72",
			parent:      "1_1_20",
			expected:    types.ChildrenRange{First: 68, Last: 72},
			expectError: false,
		},
		{
			// x=205, Lx=5 (odd→5 children), Min(L6)=333
			// levelIndex = 205 - 133 = 72
			// firstChild = (72 * 5) + 333 = 693, lastChild = 693 + 4 = 697
			name:        "L5 index 205 → children 693 to 697",
			parent:      "1_2_205",
			expected:    types.ChildrenRange{First: 693, Last: 697},
			expectError: false,
		},
		{
			// x=376, Lx=6 → leaf level, should error
			name:        "L6 index 376 is leaf → error",
			parent:      "9_1_376",
			expected:    types.ChildrenRange{},
			expectError: true,
		},
		{
			// x=1220, Lx=6 → leaf level, should error
			name:        "L6 index 1220 is leaf → error",
			parent:      "2_2_1220",
			expected:    types.ChildrenRange{},
			expectError: true,
		},
		{
			// Out of range part index → error
			name:        "out of range part index returns error",
			parent:      "1_1_9999",
			expected:    types.ChildrenRange{},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.parent.GetChildrenIndexRange()
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none; result: %+v", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("got %+v, want %+v", result, tc.expected)
			}
		})
	}
}

func TestGetParentToken(t *testing.T) {
	tests := []struct {
		name        string
		child       TokenID
		expected    string
		expectError bool
	}{
		{
			// L2 node, globalIndex=3, Min(L2)=3, Lp=1 (odd→5 children)
			// levelIndex = 3 - 3 = 0
			// parentLevelIndex = 0 / 5 = 0
			// parentGlobalIndex = Min(L1) + 0 = 1 + 0 = 1
			name:        "L2 index 3 parent is L1 index 1",
			child:       "1_1_3",
			expected:    "1_1_1",
			expectError: false,
		},
		{
			// L2 node, globalIndex=7, Min(L2)=3, Lp=1 (odd→5 children)
			// levelIndex = 7 - 3 = 4
			// parentLevelIndex = 4 / 5 = 0
			// parentGlobalIndex = 1 + 0 = 1
			name:        "L2 index 7 parent is L1 index 1",
			child:       "1_1_7",
			expected:    "1_1_1",
			expectError: false,
		},
		{
			// L2 node, globalIndex=8, Min(L2)=3, Lp=1 (odd→5 children)
			// levelIndex = 8 - 3 = 5
			// parentLevelIndex = 5 / 5 = 1
			// parentGlobalIndex = 1 + 1 = 2
			name:        "L2 index 8 parent is L1 index 2",
			child:       "1_1_8",
			expected:    "1_1_2",
			expectError: false,
		},
		{
			// L3 node, globalIndex=13, Min(L3)=13, Lp=2 (even→2 children)
			// levelIndex = 13 - 13 = 0
			// parentLevelIndex = 0 / 2 = 0
			// parentGlobalIndex = Min(L2) + 0 = 3 + 0 = 3
			name:        "L3 index 13 parent is L2 index 3",
			child:       "1_1_13",
			expected:    "1_1_3",
			expectError: false,
		},
		{
			// L3 node, globalIndex=14, Min(L3)=13, Lp=2 (even→2 children)
			// levelIndex = 14 - 13 = 1
			// parentLevelIndex = 1 / 2 = 0
			// parentGlobalIndex = 3 + 0 = 3
			name:        "L3 index 14 parent is L2 index 3",
			child:       "10_1_14",
			expected:    "10_1_3",
			expectError: false,
		},
		{
			// x=197, Lx=5, levelIndex=197-133=64, Lp=4 (even→2 children)
			// parentLevelIndex = 64 / 2 = 32
			// parentGlobalIndex = Min(L4) + 6 = 33 + 32 = 65
			name:        "L5 index 197 → parent L4 index 65",
			child:       "7_87_197",
			expected:    "7_87_65",
			expectError: false,
		},
		{
			// x=65, Lx=4, levelIndex=65-33=32, Lp=3 (odd→5 children)
			// parentLevelIndex = 32 / 5 = 6
			// parentGlobalIndex = Min(L3) + 6 = 13 + 6 = 19
			name:        "L4 index 65 → parent L3 index 19",
			child:       "32_67_65",
			expected:    "32_67_19",
			expectError: false,
		},
		{
			// L1 node, globalIndex=1, Min(L1)=1, Lp=0 (even→2 children)
			// parentGlobalIndex = 0 (whole token)
			// parent = 2_1
			name:        "L1 root node returns wholetoken",
			child:       "2_1_1",
			expected:    "2_1",
			expectError: false,
		},
		{
			// Out of range part index
			name:        "out of range part index returns error",
			child:       "25_1_9999",
			expected:    "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.child.GetParentToken()
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none; result: %+v", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("got %+v, want %+v", result, tc.expected)
			}
		})
	}
}
