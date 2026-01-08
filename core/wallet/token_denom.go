package wallet

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func IdxToDenom(idx int) (float64, error) {
	if idx < 0 {
		return 0, errors.New("idx must be non-negative")
	}

	k := idx / 2
	if idx%2 == 0 {
		return math.Pow(10, -float64(k)), nil
	}
	return 5 * math.Pow(10, -float64(k+1)), nil
}

func DenomToIdx(denom float64) (int, error) {
	if denom <= 0 {
		return 0, errors.New("denom must be positive")
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

	return 0, errors.New("denom is not part of the supported denomination set")
}

func CreateTokenDenomArr(supportedDecimalPlaces int) []string {
	arrLen := GetMaxLevel(supportedDecimalPlaces)
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

func GetMaxLevel(supportedDecimalPlaces int) int {
	return (2 * supportedDecimalPlaces) + 1
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

func IncrementTokenDenomArrayAtIndex(tokenDenomArr []string, index int, incrementValue int) error {
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

func DecrementTokenDenomArrayAtIndex(tokenDenomArr []string, index int, decrementValue int) error {
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
