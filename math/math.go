package math

import (
	"errors"
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
)

const MaxDecimalPlaces = 3

func round(f float64) int {
	return int(f + math.Copysign(0.5, f))
}

func FloatPrecision(f float64) float64 {
	output := math.Pow(10, float64(constants.MaxSupportedDecimalPlaces))
	return float64(round(f*output)) / output
}

func ZeroFloat() float64 {
	return FloatPrecision(0.0)
}

func OneFloat() float64 {
	return FloatPrecision(1.0)
}

func AddFloat(a float64, b float64) float64 {
	return FloatPrecision(a + b)
}

func ScaledMultFloatInt(a float64, b int64) (float64, error) {
	if math.IsNaN(a) || math.IsInf(a, 0) {
		return 0, errors.New("invalid float value")
	}

	scale := math.Pow10(constants.MaxSupportedDecimalPlaces)

	scaled := math.Round(a * scale)

	scaledInt := int64(scaled)

	// detect overflow
	if b != 0 && (scaledInt > math.MaxInt64/b || scaledInt < math.MinInt64/b) {
		return 0, errors.New("multiplication overflow")
	}

	resultUnits := scaledInt * b

	return float64(resultUnits) / scale, nil
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}