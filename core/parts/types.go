package parts

import (
	"fmt"
	"strings"
)

type TokenID string

func ParseTokenID(id TokenID, wholeTokenID string) (root string, path []int, err error) {
	s := string(id)

	// If this IS the whole token, no hierarchy
	if s == wholeTokenID {
		return s, []int{}, nil
	}

	// Check if the ID starts with the whole token prefix
	expectedPrefix := wholeTokenID + "_"
	if !strings.HasPrefix(s, expectedPrefix) {
		return "", nil, fmt.Errorf("token ID '%s' does not belong to whole token '%s'", s, wholeTokenID)
	}

	// Extract the part token hierarchy (everything after the whole token ID)
	hierarchySuffix := s[len(expectedPrefix):]
	parts := strings.Split(hierarchySuffix, "_")

	// Parse each segment as an integer index
	path = make([]int, len(parts))
	for i, part := range parts {
		var idx int
		_, err := fmt.Sscanf(part, "%d", &idx)
		if err != nil {
			return "", nil, fmt.Errorf("invalid token ID segment '%s': %w", part, err)
		}
		path[i] = idx
	}

	return wholeTokenID, path, nil
}

func (id TokenID) GetWholeTokenID() TokenID {
	// Walk back through parents to find the whole token
	// The whole token is when there's no more parent
	currentID := id
	for {
		parent := currentID.Parent()
		if parent == nil {
			return currentID
		}
		currentID = *parent
	}
}

func (id TokenID) Level() int {
	// Count parent hops to determine level
	level := 0
	currentID := id
	
	idElems := strings.Split(currentID.String(), "_")
	if len(idElems) == 2 {
		return 0 // Case for whole token
	}

	for {
		parent := currentID.Parent()
		if parent == nil {
			return level
		}

		level++
		currentID = *parent
	}
}

// Parent returns the parent token ID, or nil for whole tokens
func (id TokenID) Parent() *TokenID {
	s := string(id)

	// Find the last dash to get parent
	lastDash := strings.LastIndex(s, "_")
	if lastDash == -1 {
		return nil // Whole token has no parent
	}

	// Return parent hierarchical string
	parentHierarchical := s[:lastDash]
	parent := TokenID(parentHierarchical)
	return &parent
}

// Child returns the token ID for a child at the given index (1-based)
func (id TokenID) Child(index int) TokenID {
	// Create child hierarchical string by appending index
	return TokenID(fmt.Sprintf("%s-%d", string(id), index))
}

// Children returns all child token IDs for a given split factor
func (id TokenID) Children(splitFactor int) []TokenID {
	children := make([]TokenID, splitFactor)
	for i := 1; i <= splitFactor; i++ {
		children[i-1] = id.Child(i)
	}
	return children
}

// IsAncestorOf returns true if this token ID is an ancestor of other
func (id TokenID) IsAncestorOf(other TokenID) bool {
	return strings.HasPrefix(string(other), string(id)+"_")
}

// LexicalCompare compares two token IDs for left-to-right ordering
// Returns -1 if a < b, 0 if a == b, 1 if a > b
// This is format-agnostic and works by getting whole token roots and parsing with them
func (a TokenID) LexicalCompare(b TokenID) int {
	// Get whole token IDs for both
	rootA := a.GetWholeTokenID()
	rootB := b.GetWholeTokenID()

	// Compare whole tokens first
	if rootA < rootB {
		return -1
	}
	if rootA > rootB {
		return 1
	}

	// Same whole token - compare hierarchical paths
	_, pathA, _ := ParseTokenID(a, string(rootA))
	_, pathB, _ := ParseTokenID(b, string(rootB))

	minLen := len(pathA)
	if len(pathB) < minLen {
		minLen = len(pathB)
	}

	for i := 0; i < minLen; i++ {
		if pathA[i] < pathB[i] {
			return -1
		}
		if pathA[i] > pathB[i] {
			return 1
		}
	}

	if len(pathA) < len(pathB) {
		return -1
	}
	if len(pathA) > len(pathB) {
		return 1
	}

	return 0
}

// String returns the string representation
func (id TokenID) String() string {
	return string(id)
}

type SplitOp struct {
	TokenID            TokenID
	ChildrenToTransfer []int // Which child indices (1-based) go to recipient
	ChildrenToKeep     []int // Which child indices stay with sender
}
