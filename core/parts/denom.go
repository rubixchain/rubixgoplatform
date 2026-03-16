package parts

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
)

// get level, token number and global index from the RBT token id
func (id TokenID) GetRbtIDFields() (types.RbtIDElements, error) {
	var err error
	rbtElems := types.RbtIDElements{}

	idElems := strings.Split(id.String(), "_")
	rbtElems.Level, err = strconv.Atoi(idElems[0])
	if err != nil {
		return types.RbtIDElements{}, fmt.Errorf("failed to convert level into int for rbt: %s, error: %v", id, err)
	}
	rbtElems.TokenNumber, err = strconv.Atoi(idElems[1])
	if err != nil {
		return types.RbtIDElements{}, fmt.Errorf("failed to convert token number into int for rbt: %s, error: %v", id, err)
	}

	switch len(idElems) {
	case 2:
		rbtElems.GlobalIndex = 0 // Case for whole token
	case 3:
		rbtElems.GlobalIndex, err = strconv.Atoi(idElems[2]) // Case for part token
		if err != nil {
			return types.RbtIDElements{}, fmt.Errorf("failed to convert global index into int for rbt: %s, error: %v", id, err)
		}
	default:
		return types.RbtIDElements{}, fmt.Errorf("invalid token id format for rbt: %s, id elements should be 2 (whole) or 3 (part)", id)
	}
	return rbtElems, nil
}

// GetMaxDenomTreeLevel gets the max level of a Denom Tree
func GetMaxDenomTreeLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

func getLowestPossibleDenom() float64 {
	return rubixmath.FloatPrecision(math.Pow10(-constants.MaxSupportedDecimalPlaces))
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

// getSuffixProduct returns the product of numChildren(j) for j in [from, to).
// numChildren(j) = 2 if j is even, 5 if j is odd.
func getSuffixProduct(from, to int) int {
	product := 1
	for j := from; j < to; j++ {
		if j%2 == 0 {
			product *= 2
		} else {
			product *= 5
		}
	}
	return product
}

func IndexedToHierarchical(indexedToken string) (TokenID, error) {
	parts := strings.Split(indexedToken, "_")
	if len(parts) < 3 {
		if len(parts) == 2 {
			// This is the whole token case
			return TokenID(indexedToken), nil
		}
		return "", fmt.Errorf("invalid indexed token format: expected at least 3 parts, got %d", len(parts))
	}

	// Extract prefix (TokenLevel_TokenNumber)
	prefix := parts[0] + "_" + parts[1]

	// Parse the indexed number
	var tokenId int
	_, err := fmt.Sscanf(parts[2], "%d", &tokenId)
	if err != nil {
		return "", fmt.Errorf("invalid indexed number: %s", parts[2])
	}

	path := GetTokenPath(tokenId)
	// If path is empty, return just the prefix (whole token)
	if path == "" {
		return TokenID(prefix), nil
	}

	// Return full hierarchical form
	return TokenID(prefix + "_" + path), nil
}

// GetTokenPath converts a BFS tokenId to its hierarchical path representation.
// Returns a string like "1_2" (just the path portion, without the token prefix).
func GetTokenPath(tokenId int) string {
	maxLevel := GetMaxDenomTreeLevel() - 1

	// Root node (whole token)
	if tokenId == 0 {
		return ""
	}

	// Find which level this tokenId belongs to
	level := 0
	for level < maxLevel {
		if tokenId < getLevelStart(level+1) {
			break
		}
		level++
	}

	// Position within this level (0-indexed)
	pos := tokenId - getLevelStart(level)

	// Decode the path using mixed-radix arithmetic
	path := make([]int, level)
	for i := 0; i < level; i++ {
		sp := getSuffixProduct(i+1, level)
		path[i] = pos/sp + 1
		pos = pos % sp
	}

	// Convert path to string
	pathStr := make([]string, level)
	for i := 0; i < level; i++ {
		pathStr[i] = fmt.Sprintf("%d", path[i])
	}

	return strings.Join(pathStr, "_")
}

func HeirarchicalToIndexed(heirarchicalID TokenID) (string, error) {
	parts := strings.Split(heirarchicalID.String(), "_")
	if len(parts) < 3 {
		if len(parts) == 2 {
			// This is the whole token case
			return string(heirarchicalID), nil
		}
		return "", fmt.Errorf("invalid hierarchical token format: expected at least 3 parts, got %d", len(parts))
	}

	// Extract prefix (TokenLevel_TokenNumber)
	prefix := parts[0] + "_" + parts[1]

	// Extract hierarchical path (everything after prefix)
	pathParts := parts[2:]
	pathStr := strings.Join(pathParts, "_")

	indexedToken, err := GetTokenIdFromPath(pathStr)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s_%d", prefix, indexedToken), nil
}

// GetTokenIdFromPath converts a hierarchical path string to its BFS tokenId.
// Takes a path like "34_2345_1_2" (with prefix) or "1_2" (without prefix).
func GetTokenIdFromPath(pathStr string) (int, error) {
	maxDepthWithoutRoot := GetMaxDenomTreeLevel() - 1

	parts := strings.Split(pathStr, "_")

	// Auto-detect prefix: check if first part is a valid position (1 or 2)
	// If not, assume first 2 parts are prefix (NetworkLevel_TokenNumber)
	if len(parts) >= 3 {
		var firstVal int
		fmt.Sscanf(parts[0], "%d", &firstVal)
		if firstVal > 2 {
			// First part is not a valid position, strip prefix
			parts = parts[2:]
			pathStr = strings.Join(parts, "_")
		}
	}

	// Root case
	if pathStr == "0" {
		return 0, fmt.Errorf("unexpected case: whole token found for path: %v", pathStr)
	}

	if len(parts) > maxDepthWithoutRoot {
		return 0, fmt.Errorf("path length %d exceeds MaxDepth %d", len(parts), maxDepthWithoutRoot)
	}

	path := make([]int, len(parts))
	for i := 0; i < len(parts); i++ {
		_, err := fmt.Sscanf(parts[i], "%d", &path[i])
		if err != nil {
			return 0, fmt.Errorf("invalid number at depth %d: %s", i, parts[i])
		}
	}

	n := len(path)

	// BFS index = level_start(n) + position_within_level
	levelStart := getLevelStart(n)
	pos := 0
	for i := 0; i < n; i++ {
		child := path[i]

		var numChildren int
		if i%2 == 0 {
			numChildren = 2
		} else {
			numChildren = 5
		}

		if child < 1 || child > numChildren {
			return 0, fmt.Errorf("invalid child %d at depth %d", child, i)
		}

		sp := getSuffixProduct(i+1, n)
		pos += (child - 1) * sp
	}

	return levelStart + pos, nil
}

func GetSplitAndNonsplitTokenDenom(
	inputTokenDenom map[types.DenomValue]types.DenomCount, 
	transferAmount float64,
) (targetDenomArr map[types.DenomValue]types.DenomCount, updatedDenomArr map[types.DenomValue]types.DenomCount, remaining float64, err error) {
		
	targetDenomArr = make(map[types.DenomValue]types.DenomCount)
	updatedDenomArr = make(map[types.DenomValue]types.DenomCount)

	for k, v := range inputTokenDenom {
		updatedDenomArr[k] = v
	}

	remaining = rubixmath.FloatPrecision(transferAmount)

	for denomValue, denomCount := range updatedDenomArr {
		if remaining <= 0 {
			break
		}

		maxByTarget := int(math.Floor(remaining / denomValue))

		canTake := rubixmath.Min(int(denomCount), maxByTarget)

		if canTake > 0 {
			amount := float64(canTake) * denomValue

			targetDenomArr[denomValue] = int64(canTake)
			updatedDenomArr[denomValue] -= int64(canTake)

			remaining = rubixmath.FloatPrecision(remaining - amount)
		}
	}

	return targetDenomArr, updatedDenomArr, remaining, nil
}

// ********* replacement for : getLevelStart()
// LevelMin returns the minimum global index for a given level (1-indexed).
func LevelMin(level int) int {
	return constants.TreeLevelRanges[level][0]
}

// GetTreeLevelFromGlobalIndex returns the tree level (1–6) for a given global index x.
func GetTreeLevelFromGlobalIndex(x int) (int, error) {
	for level, r := range constants.TreeLevelRanges {
		if x >= r[0] && x <= r[1] {
			return level, nil // levels are 1-indexed
		}
	}
	return 0, fmt.Errorf("global index %d is out of range (valid: 1–1332)", x)
}

// ********* replacement for : getSuffixProduct()
// GetNumberOfChildren returns the number of children for a node at parentLevel.
// n = parentLevel % 2:  n==0 → 2 children,  n==1 → 5 children.
func GetNumberOfChildren(parentLevel int) int {
	if parentLevel%2 == 0 {
		return 2 // when parent level is even
	}
	return 5 // when parent level is odd
}

// GetParentToken derives the parent TokenID from a child's global index.
//
// Steps:
//  1. x          = globalIndex
//  2. Lx         = GetTreeLevelFromGlobalIndex(x)
//  3. childLevelIndex = x - Min(Lx)
//  4. Lp         = Lx - 1                          (parent level)
//  5. numChildren= GetNumberOfChildren(Lp)
//  6. parentLevelIndex = childLevelIndex / numChildren   (integer division)
//  7. parentGlobalIndex= Min(Lp) + parentLevelIndex
func (id TokenID) GetParentToken() (string, error) {
	child, err := id.GetRbtIDFields()
	if err != nil {
		return "", err
	}

	// x: child global index
	x := child.GlobalIndex

	// lx: child level on tree
	lx, err := GetTreeLevelFromGlobalIndex(x)
	if err != nil {
		return "", fmt.Errorf("invalid token: %s, error: %v", id, err)
	}
	// if child has tree level 1, then the parent is the whole token, which does not have any global index in tokenID
	if lx == 1 {
		parentToken := fmt.Sprintf("%d_%d", child.Level, child.TokenNumber)
		return parentToken, nil
	}

	childLevelIndex := x - LevelMin(lx)

	// lp: parent level on tree
	lp := lx - 1
	numChildren := GetNumberOfChildren(lp)

	// parent position in the level
	parentLevelIndex := childLevelIndex / numChildren
	parentGlobalIndex := LevelMin(lp) + parentLevelIndex

	return fmt.Sprintf("%d_%d_%d", child.Level, child.TokenNumber, parentGlobalIndex), nil
}

// GetChildrenIndexRange returns the global-index range [first, last] of the children
// of the node identified by parentGlobalIndex.
//
// Steps:
//  1. x          = parentGlobalIndex
//  2. Lx         = GetTreeLevelFromTreeMap(x)
//  3. levelIndex = x - Min(Lx)
//  4. numChildren= GetNumberOfChildren(Lx)
//  5. firstChild = (levelIndex * numChildren) + Min(Lx+1)
//  6. lastChild  = firstChild + (numChildren - 1)
func (id TokenID) GetChildrenIndexRange() (types.ChildrenRange, error) {
	parent, err := id.GetRbtIDFields()
	if err != nil {
		return types.ChildrenRange{}, err
	}

	// global index of parent token
	x := parent.GlobalIndex

	// tree level of parent token
	lx, err := GetTreeLevelFromGlobalIndex(x)
	if err != nil {
		return types.ChildrenRange{}, err
	}
	if lx == 6 {
		return types.ChildrenRange{}, fmt.Errorf("global index %d is at level 6 — leaf nodes have no children", x)
	}

	// parent position in the level
	levelIndex := x - LevelMin(lx)
	numChildren := GetNumberOfChildren(lx)
	firstChild := (levelIndex * numChildren) + LevelMin(lx+1)
	lastChild := firstChild + (numChildren - 1)

	return types.ChildrenRange{First: firstChild, Last: lastChild}, nil
}
