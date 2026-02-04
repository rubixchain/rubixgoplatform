package parts

import (
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
)

// GetMaxDenomTreeLevel gets the max level of a Denom Tree
func GetMaxDenomTreeLevel() int {
	return (2 * constants.MaxSupportedDecimalPlaces) + 1
}

func getLowestPossibleDenom() float64 {
	return rubixmath.FloatPrecision(math.Pow10(-constants.MaxSupportedDecimalPlaces))
}
