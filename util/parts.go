package util

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types"
)

type TokenID string

func ParseTokenID(id TokenID) (path []int, err error) {
	tokenElems, err := GetRbtIDElements(id.String())
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

		parentElems, err := GetRbtIDElements(parent)
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
	tokenElems, err := GetRbtIDElements(id.String())
	if err != nil {
		return "", fmt.Errorf("GetWholeTokenID: failed to extract token id elements, err: %w", err)
	}
	// level_tokenNumber is the corresponding whole token of any part token
	return TokenID(fmt.Sprintf("%d_%d", tokenElems.TokenLevel, tokenElems.TokenNumber)), nil
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
	tokenElems, err := GetRbtIDElements(string(id))
	if err != nil {
		return ""
	}
	partIndex := childrenRange.First + (childPosition - 1)
	// Create child hierarchical string by appending index
	return TokenID(fmt.Sprintf("%d_%d_%d", tokenElems.TokenLevel, tokenElems.TokenNumber, partIndex))
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

// GetParentToken derives the parent TokenID from a child's part index.
//
// Steps:
//  1. x          = PartIndex
//  2. Lx         = GetTreeLevelFromPartIndex(x)
//  3. childLevelIndex = x - Min(Lx)
//  4. Lp         = Lx - 1                          (parent level)
//  5. numChildren= GetNumberOfChildren(Lp)
//  6. parentLevelIndex = childLevelIndex / numChildren   (integer division)
//  7. parentPartIndex= Min(Lp) + parentLevelIndex
func (id TokenID) GetParentToken() (string, error) {
	child, err := GetRbtIDElements(string(id))
	if err != nil {
		return "", err
	}

	// x: child part index
	x := child.PartIndex
	if x == 0 {
		//PartIndex is zero only for whole token, which has no parent
		return "", nil
	}

	// lx: child level on tree
	lx, err := GetTreeLevelFromPartIndex(x)
	if err != nil {
		return "", fmt.Errorf("invalid token: %s, error: %v", id, err)
	}
	// if child has tree level 1, then the parent is the whole token, which does not have any part index in tokenID
	if lx == 1 {
		parentToken := fmt.Sprintf("%d_%d", child.TokenLevel, child.TokenNumber)
		return parentToken, nil
	}

	childLevelIndex := x - getLevelStart(lx)

	// lp: parent level on tree
	lp := lx - 1
	numChildren := GetNumberOfChildren(lp)

	// parent position in the level
	parentLevelIndex := childLevelIndex / numChildren
	parentPartIndex := getLevelStart(lp) + parentLevelIndex

	return fmt.Sprintf("%d_%d_%d", child.TokenLevel, child.TokenNumber, parentPartIndex), nil
}

// GetHierarchy returns the list of ancestor token IDs from the whole token down to the immediate parent.
// It returns list of tokenIDs in the order: [immediate parent, grandparent, ..., whole token]
func (id TokenID) GetHierarchy() ([]string, error) {
	var hierarchy []string = make([]string, 0)

	current := id
	for {
		parent, err := current.GetParentToken()
		if err != nil {
			return nil, fmt.Errorf("error getting parent for token %s: %v", current, err)
		}
		if parent == "" {
			break
		}
		hierarchy = append(hierarchy, parent)
		current = TokenID(parent)
	}

	return hierarchy, nil
}


// GetChildrenIndexRange returns the part-index range [first, last] of the children
// of the node identified by parentPartIndex.
//
// Steps:
//  1. x          = parentPartIndex
//  2. Lx         = GetTreeLevelFromTreeMap(x)
//  3. levelIndex = x - Min(Lx)
//  4. numChildren= GetNumberOfChildren(Lx)
//  5. firstChild = (levelIndex * numChildren) + Min(Lx+1)
//  6. lastChild  = firstChild + (numChildren - 1)
func (id TokenID) GetChildrenIndexRange() (types.ChildrenRange, error) {
	parent, err := GetRbtIDElements(string(id))
	if err != nil {
		return types.ChildrenRange{}, err
	}

	// part index of parent token
	x := parent.PartIndex

	// tree level of parent token
	lx, err := GetTreeLevelFromPartIndex(x)
	if err != nil {
		return types.ChildrenRange{}, err
	}
	if lx == 6 {
		return types.ChildrenRange{}, fmt.Errorf("part index %d is at level 6 — leaf nodes have no children", x)
	}

	// parent position in the level
	levelIndex := x - getLevelStart(lx)
	numChildren := GetNumberOfChildren(lx)
	firstChild := (levelIndex * numChildren) + getLevelStart(lx+1)
	lastChild := firstChild + (numChildren - 1)

	return types.ChildrenRange{First: firstChild, Last: lastChild}, nil
}



// getLevelStart returns the BFS start index for the given level.
// Level 0 (root/whole token) starts at 0. Level 1 starts at 1, etc.
func getLevelStart(level int) int {
	start := 0
	count := 1 // nodes at level 0
	for l := 0; l < level; l++ {
		start += count
		if l%2 == 0 {
			count *= 2
		} else {
			count *= 5
		}
	}
	return start
}
