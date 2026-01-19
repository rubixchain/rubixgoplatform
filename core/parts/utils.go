package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/token"
)

func getTokenType(isTestnet bool, tokenValue float64) int {
	if isTestnet {
		if tokenValue == 1 {
			return token.TestTokenType
		} else {
			return token.TestPartTokenType
		}
	} else {
		if tokenValue == 1 {
			return token.RBTTokenType
		} else {
			return token.PartTokenType
		}
	}
}

func scaledFloatDiv(a float64, b float64) (int, error) {
	scaledA := scaleFloat(a)
	scaledB := scaleFloat(b)

	if scaledB == 0 {
		return 0, fmt.Errorf("floatDiv: right operand found to be zero")
	}

	result := scaledA / scaledB
	return result, nil
}

func scaledMod(a float64, b float64) (int, error) {
	scaledA := scaleFloat(a)
	scaledB := scaleFloat(b)

	if scaledB == 0 {
		return 0, fmt.Errorf("floatModulo: right operand found to be zero")
	}

	result := scaledA % scaledB
	return result, nil
}

func scaleFloat(f float64) int {
	expVal := math.Pow10(MaxSupportedDecimalPlaces)

	return int(FloatPrecision(f * expVal))
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func FloatPrecision(num float64) float64 {
	output := math.Pow(10, float64(MaxSupportedDecimalPlaces))
	return float64(round(num*output)) / output
}

func MaxTokensAtLevel(level int) int {
	if level%2 == 1 {
		return 2
	}

	return 5
}
