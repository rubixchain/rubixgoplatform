package math

import (
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
)

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