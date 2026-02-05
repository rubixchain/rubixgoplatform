package core

import (
	"math"
)

func MinDecimalValue(num int) float64 {
	return math.Pow(10, float64(-num))
}
func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}
func Ceilround(num float64) int {
	return int(math.Ceil(num))
}
func floatPrecision(num float64, precision int) float64 {
	precision = MaxDecimalPlaces
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}
func CeilfloatPrecision(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(Ceilround(num*output)) / output
}


