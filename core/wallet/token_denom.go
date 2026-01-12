package wallet

import (
	"errors"
	"math"
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

func GetMaxLevel(supportedDecimalPlaces int) int {
	return (2 * supportedDecimalPlaces) + 1
}
