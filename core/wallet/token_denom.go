package wallet

import (
	"fmt"
	"math"
	"strconv"
	"strings"

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

func CreateTokenDenomArr() []string {
	arrLen := GetMaxLevel()
	tokenDenomArr := make([]string, arrLen)

	for i := range tokenDenomArr {
		tokenDenomArr[i] = "0"
	}
	return tokenDenomArr
}

func GetTokenDenomCount(tokenDenomArr []string, index int) (int, error) {
	if index > len(tokenDenomArr)-1 {
		return 0, fmt.Errorf("GetTokenDenomCount: index provided is out of bound, provided value: %v", index)
	}

	tokenDenomArrCountStr := tokenDenomArr[index]

	tokenDenomArrCount, err := strconv.ParseInt(tokenDenomArrCountStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("GetTokenDenomCount: failed to parse tokenDenomArrCountStr, err: %v", err)
	}

	return int(tokenDenomArrCount), nil
}

func GetMaxLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

func GetTokenDenomStrFromArr(tokenDenomArr []string) string {
	return strings.Join(tokenDenomArr, ",")
}

func GetTokenDenomArrFromStr(tokenDenomStr string) []string {
	return strings.Split(tokenDenomStr, ",")
}

func UpdateTokenDenomStrIndex(tokenDenomStr string, index int, value int) (string, error) {
	tokenDenomArr := strings.Split(tokenDenomStr, ",")

	err := UpdateTokenDenomArrayIndex(tokenDenomArr, index, value)
	if err != nil {
		return "", fmt.Errorf("error occured while updating token denom str, err: %v", err)
	}

	return strings.Join(tokenDenomArr, ","), nil
}

func UpdateTokenDenomArrayIndex(tokenDenomArr []string, index int, value int) error {
	if index > len(tokenDenomArr)-1 {
		return fmt.Errorf("invalid index value: %v, array length: %v", index, len(tokenDenomArr))
	}

	denomValueStr := strconv.FormatInt(int64(value), 10)
	tokenDenomArr[index] = denomValueStr
	return nil
}

func SetTokenDenomCountAtIndex(tokenDenomArr []string, index int, setValue int) error {
	if index > len(tokenDenomArr)-1 {
		return fmt.Errorf("invalid index value: %v, array length: %v", index, len(tokenDenomArr))
	}

	if setValue < 0 {
		return fmt.Errorf("value passed for set must be greater than 0, value provided: %v", setValue)
	}

	updatedDenomCount := int64(setValue)

	updatedDenomCountStr := strconv.FormatInt(updatedDenomCount, 10)
	tokenDenomArr[index] = updatedDenomCountStr
	return nil
}

func IncrementTokenDenomCountAtIndex(tokenDenomArr []string, index int, incrementValue int) error {
	if index > len(tokenDenomArr)-1 {
		return fmt.Errorf("invalid index value: %v, array length: %v", index, len(tokenDenomArr))
	}

	// sanity check for increment value
	if incrementValue <= 0 {
		return fmt.Errorf("value passed for increment must be greater than 0, value provided: %v", incrementValue)
	}

	existingDenomCount, err := strconv.ParseInt(tokenDenomArr[index], 10, 64)
	if err != nil {
		return fmt.Errorf("unable to parse int for index %v of token denom arr, err: %v", index, err)
	}

	updatedDenomCount := existingDenomCount + int64(incrementValue)

	updatedDenomCountStr := strconv.FormatInt(updatedDenomCount, 10)
	tokenDenomArr[index] = updatedDenomCountStr
	return nil
}

func DecrementTokenDenomCountAtIndex(tokenDenomArr []string, index int, decrementValue int) error {
	if index > len(tokenDenomArr)-1 {
		return fmt.Errorf("invalid index value: %v, array length: %v", index, len(tokenDenomArr))
	}

	// sanity check for decrement value
	if decrementValue <= 0 {
		return fmt.Errorf("value passed for decrement must be greater than 0, value provided: %v", decrementValue)
	}

	existingDenomCount, err := strconv.ParseInt(tokenDenomArr[index], 10, 64)
	if err != nil {
		return fmt.Errorf("unable to parse int for index %v of token denom arr, err: %v", index, err)
	}

	updatedDenomCount := existingDenomCount - int64(decrementValue)

	if updatedDenomCount < 0 {
		return fmt.Errorf("unexpected error: updated Denom count value is less than 0")
	}

	updatedDenomCountStr := strconv.FormatInt(updatedDenomCount, 10)
	tokenDenomArr[index] = updatedDenomCountStr
	return nil
}

// GetTokenDenomArrayWithoutSplit collects enough tokens, that doesn't
// require splitting, for token transfer
//
// Returns:
//   - (targetDenomArr, []string): Target Denom Array where tokens can be taken up without split
//   - (updatedDenomArr, []string): Updated Denom Array after taking away from Target Denom Array
//   - (remaining, float64): Remaining transfer value after targetDenomArr is built
//   - (err, error): Returns any error
func GetTokenDenomArrayWithoutSplit(
	balanceDenomArr []string, 
	transferAmount float64,
) (targetDenomArr []string, updatedDenomArr []string, remaining float64, err error) {
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

		// normalize before division
		remaining = rubixmath.FloatPrecision(remaining)

		maxByTarget := int(math.Floor(remaining / denomValue))

		updatedDenomArrDenomCountAtLevel, err := GetTokenDenomCount(updatedDenomArr, level)
		if err != nil {
			return nil, nil, rubixmath.ZeroFloat(), 
				fmt.Errorf("GetTokenDenomArrayWithoutSplit: failed to parse denom count at level: %v, err: %v", level, err)
		}

		canTake := min(updatedDenomArrDenomCountAtLevel, maxByTarget)

		if canTake > 0 {
			amount := float64(canTake) * denomValue

			errSet := SetTokenDenomCountAtIndex(targetDenomArr, level, canTake)
			if errSet != nil {
				return nil, nil, rubixmath.ZeroFloat(), 
					fmt.Errorf("GetTokenDenomArrayWithoutSplit: failed to set denom count at level: %v, err: %v", level, errSet)
			}
			
			errDecrement := DecrementTokenDenomCountAtIndex(updatedDenomArr, level, canTake)
			if errDecrement != nil {
				return nil, nil, rubixmath.ZeroFloat(),
					fmt.Errorf("GetTokenDenomArrayWithoutSplit: failed to decrement denom count at level: %v, err: %v", level, errDecrement)
			}

			total = rubixmath.FloatPrecision(total + amount)
			remaining = rubixmath.FloatPrecision(remaining - amount)
		}
	}

	return targetDenomArr, updatedDenomArr, remaining, nil
}
