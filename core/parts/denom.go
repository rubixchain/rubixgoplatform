package parts

import (
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types"
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

		maxByTarget := int(rubixmath.ScaledFloatDiv(remaining, denomValue))
		
		canTake := rubixmath.Min(int(denomCount), maxByTarget)

		if canTake > 0 {
			amount := rubixmath.FloatPrecision(float64(canTake) * denomValue)

			targetDenomArr[denomValue] = int64(canTake)
			updatedDenomArr[denomValue] -= int64(canTake)

			remaining = rubixmath.FloatPrecision(remaining - amount)
		}
	}

	return targetDenomArr, updatedDenomArr, remaining, nil
}
