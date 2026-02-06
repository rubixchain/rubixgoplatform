package wallet

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
)

func LevelToDenom(level int) (float64, error) {
	if level < 0 {
		return 0, fmt.Errorf("LevelToDenom: level cannot be non negative, provided level: %v", level)
	}

	k := level / 2
	if level%2 == 0 {
		return math.Pow(10, -float64(k)), nil
	}
	return 5 * math.Pow(10, -float64(k+1)), nil
}

func DenomToLevel(denom float64) (int, error) {
	if denom <= 0 {
		return 0, fmt.Errorf("DenomToLevel: denom must be positive, provided denom: %v", denom)
	}

	exp := math.Floor(math.Log10(denom))
	mantissa := denom / math.Pow(10, exp)

	const eps = 1e-9
	if math.Abs(mantissa-1.0) < eps {
		return int(-2 * exp), nil
	}
	if math.Abs(mantissa-5.0) < eps {
		return int(-2*exp - 1), nil
	}

	return 0, fmt.Errorf("DenomToLevel: denom %v is not part of the supported denomination set", denom)
}

func CreateTokenDenomArr() []int {
	arrLen := GetMaxLevel()
	tokenDenomArr := make([]int, arrLen)

	return tokenDenomArr
}

func GetMaxLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

// GetTokenDenomArrayWithoutSplit collects enough tokens, that doesn't
// require splitting, for token transfer
//
// Returns:
//   - (targetDenomArr, []int): Target Denom Array where tokens can be taken up without split
//   - (updatedDenomArr, []int): Updated Denom Array after taking away from Target Denom Array
//   - (remaining, float64): Remaining transfer value after targetDenomArr is built
//   - (err, error): Returns any error
func GetTokenDenomArrayWithoutSplit(
	balanceDenomArr []int,
	transferAmount float64,
) (targetDenomArr []int, updatedDenomArr []int, remaining float64, err error) {
	if len(balanceDenomArr) != GetMaxLevel() {
		return nil, nil, rubixmath.ZeroFloat(), fmt.Errorf(
			"GetTokenDenomArrayWithoutSplit: unexpected error, balanceDenomArray size is not as expected, expected size: %v, received size: %v",
			GetMaxLevel(),
			len(balanceDenomArr),
		)
	}

	targetDenomArr = CreateTokenDenomArr()
	updatedDenomArr = CreateTokenDenomArr()
	copy(updatedDenomArr, balanceDenomArr)

	remaining = rubixmath.FloatPrecision(transferAmount)
	total := rubixmath.FloatPrecision(0.0)

	for level := 0; level < len(updatedDenomArr); level++ {
		if remaining <= 0 {
			break
		}

		denomValue, err := LevelToDenom(level)
		if err != nil {
			return nil, nil, rubixmath.ZeroFloat(),
				fmt.Errorf("GetTokenDenomArrayWithoutSplit: failed to get denom for level: %v, err: %v", level, err)
		}

		maxByTarget := int(math.Floor(remaining / denomValue))

		updatedDenomArrDenomCountAtLevel := updatedDenomArr[level]

		canTake := min(updatedDenomArrDenomCountAtLevel, maxByTarget)

		if canTake > 0 {
			amount := float64(canTake) * denomValue

			targetDenomArr[level] = canTake

			updatedDenomArr[level] -= canTake

			total = rubixmath.FloatPrecision(total + amount)
			remaining = rubixmath.FloatPrecision(remaining - amount)
		}
	}

	return targetDenomArr, updatedDenomArr, remaining, nil
}

// Check if token denom array is empty
// The following denom arrays qualifies as empty.
//
// - []
//
// - ["0","0","0".....n supported levels]
func CheckEmptyTokenDenomArr(denomArr []int) bool {
	if len(denomArr) == 0 {
		return true
	}

	zeroCount := 0

	for _, elem := range denomArr {
		if elem == 0 {
			zeroCount++
		}
	}

	return zeroCount == len(denomArr)
}
