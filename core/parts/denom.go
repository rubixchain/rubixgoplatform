package parts

import (
	"fmt"
	"math"
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

// GetMaxDenomTreeLevel gets the max level of a Denom Tree
func GetMaxDenomTreeLevel() int {
	return (2 * MaxSupportedDecimalPlaces) + 1
}

func getLowestPossibleDenom() float64 {
	return FloatPrecision(math.Pow10(-MaxSupportedDecimalPlaces))
}