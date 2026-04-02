package parts

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/util"
)

type TokenID string

func NewTokenIDFromString(s string) TokenID {
	return TokenID(s)
}

func ParseTokenID(id TokenID) (path []int, err error) {
	tokenElems, err := util.GetRbtIDElements(id.String())
	if err != nil {
		return nil, fmt.Errorf("ParseTokenID: %w", err)
	}
	
	// id is root token, and root has no path
	if tokenElems.PartIndex == 0 {
		return []int{}, nil
	}

	// walk up the tree, and at each step compute the node's
	// position among its siblings before moving to the parent.
	// We collect positions bottom-up, then reverse at the end.
	current := tokenElems.PartIndex
	currentToken := id
	for current != 0 {
		// get parent token
		parent, err := currentToken.GetParentToken()
		if err != nil {
			return nil, fmt.Errorf("ParseTokenID: error getting parent for index %d: %w", current, err)
		}
		// get parent's children range to find siblings
		childrenRanges, err := TokenID(parent).GetChildrenIndexRange()
		if err != nil {
			return nil, fmt.Errorf("ParseTokenID: error getting children for parent %v: %w", parent, err)
		}

		// 1-indexed position among siblings
		position := (current - childrenRanges.First) + 1
		path = append(path, position)

		parentElems, err := util.GetRbtIDElements(parent) 
		current = parentElems.PartIndex
		currentToken = TokenID(parent)
	}
	// ancestors = [globalIndex, ..., L2 node, L1 node]
	// reverse: collected bottom-up, need top-down (L1 first)
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path, nil
}

// GetWholeTokenID returns corresponding whole token pf the input token id
func (id TokenID) GetWholeTokenID() (TokenID, error) {
	// remove part index from token id, if any
	// and that is the whole token
	tokenElems, err := util.GetRbtIDElements(id.String())
	if err != nil {
		return "", fmt.Errorf("GetWholeTokenID: failed to extract token id elements, err: %w", err)
	}
	// level_tokenNumber is the corresponding whole token of any part token
	return TokenID(fmt.Sprintf("%d_%d", tokenElems.Level, tokenElems.TokenNumber)), nil
}

// Child returns the token ID for a child at the given child-position (1-based)
func (id TokenID) Child(childPosition int) TokenID {
	// get children range of the tokenID
	childrenRange, err := id.GetChildrenIndexRange()
	if err != nil {
		return ""
	}
	// index is child position in the children range
	// first-child-index + (required-child-position - 1) = required-child-index
	tokenElems, err := util.GetRbtIDElements(string(id))
	if err != nil {
		return ""
	}
	partIndex := childrenRange.First + (childPosition - 1)
	// Create child hierarchical string by appending index
	return TokenID(fmt.Sprintf("%d_%d_%d", tokenElems.Level, tokenElems.TokenNumber, partIndex))
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
	rootA, _ := a.GetWholeTokenID()
	rootB, _ := b.GetWholeTokenID()

	// Compare whole tokens first
	if rootA < rootB {
		return -1
	}
	if rootA > rootB {
		return 1
	}

	// Same whole token - compare hierarchical paths
	pathA, _ := ParseTokenID(a)
	pathB, _ := ParseTokenID(b)

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
