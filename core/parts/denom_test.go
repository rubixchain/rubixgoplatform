package parts

import (
	"testing"
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
