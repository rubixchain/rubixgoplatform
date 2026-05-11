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

// MaxSupportedDecimalPlaces returns the maximum number of decimal places
// allowed for a Rubix transfer amount.
func MaxSupportedDecimalPlaces() int {
	return constants.MaxSupportedDecimalPlaces
}

// MinTransferAmount returns the smallest amount that can be transferred in
// Rubix, i.e. 10^-MaxSupportedDecimalPlaces (0.001 at 3 decimal places).
func MinTransferAmount() float64 {
	return FloatPrecision(math.Pow10(-constants.MaxSupportedDecimalPlaces))
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

func ScaledFloatDiv(a float64, b float64) float64 {
	if b == 0 {
		return FloatPrecision(0)
	}

	scale := math.Pow10(constants.MaxSupportedDecimalPlaces)

	scaledA := math.Round(a * scale)
	scaledB := math.Round(b * scale)

	floatDiv := scaledA / scaledB

	return FloatPrecision(floatDiv)
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

	return ScaledFloatDiv(float64(resultUnits), scale), nil
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func SplitFloat(f float64, decimals int) (int64, float64) {
	// Exact base-10 scale
	scale := float64(math.Pow10(decimals))

	// Normalize to fixed precision
	normalized := math.Round(f*scale) / scale

	// Integer part
	ipart := int64(normalized)

	// Fractional part, clamped to same precision
	fpart := math.Round((normalized-float64(ipart))*scale) / scale

	return ipart, fpart
}
