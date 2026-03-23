package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
)

// GetMaxDenomTreeLevel gets the max level of a Denom Tree
func GetMaxDenomTreeLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

func getLowestPossibleDenom() float64 {
	return rubixmath.FloatPrecision(math.Pow10(-(constants.MaxSupportedDecimalPlaces)))
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
	child, err := util.GetRbtIDElements(string(id))
	if err != nil {
		return "", err
	}

	// x: child part index
	x := child.PartIndex

	// lx: child level on tree
	lx, err := util.GetTreeLevelFromPartIndex(x)
	if err != nil {
		return "", fmt.Errorf("invalid token: %s, error: %v", id, err)
	}
	// if child has tree level 1, then the parent is the whole token, which does not have any part index in tokenID
	if lx == 1 {
		parentToken := fmt.Sprintf("%d_%d", child.Level, child.TokenNumber)
		return parentToken, nil
	}

	childLevelIndex := x - getLevelStart(lx)

	// lp: parent level on tree
	lp := lx - 1
	numChildren := util.GetNumberOfChildren(lp)

	// parent position in the level
	parentLevelIndex := childLevelIndex / numChildren
	parentPartIndex := getLevelStart(lp) + parentLevelIndex

	return fmt.Sprintf("%d_%d_%d", child.Level, child.TokenNumber, parentPartIndex), nil
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
	parent, err := util.GetRbtIDElements(string(id))
	if err != nil {
		return types.ChildrenRange{}, err
	}

	// part index of parent token
	x := parent.PartIndex

	// tree level of parent token
	lx, err := util.GetTreeLevelFromPartIndex(x)
	if err != nil {
		return types.ChildrenRange{}, err
	}
	if lx == 6 {
		return types.ChildrenRange{}, fmt.Errorf("part index %d is at level 6 — leaf nodes have no children", x)
	}

	// parent position in the level
	levelIndex := x - getLevelStart(lx)
	numChildren := util.GetNumberOfChildren(lx)
	firstChild := (levelIndex * numChildren) + getLevelStart(lx+1)
	lastChild := firstChild + (numChildren - 1)

	return types.ChildrenRange{First: firstChild, Last: lastChild}, nil
}
