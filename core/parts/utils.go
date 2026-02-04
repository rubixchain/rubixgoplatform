package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
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
	expVal := math.Pow10(constants.MaxSupportedDecimalPlaces)

	return int(rubixmath.FloatPrecision(f * expVal))
}


func MaxTokensAtLevel(level int) int {
	if level%2 == 1 {
		return 2
	}

	return 5
}

// GetTokenValueFromHierarchicalID fetches the token value by
// looking at the IPFS content of a Token.
//
// Make sure to pass the IPFS content, and not the IPFS ID of the token
func GetTokenValueFromHierarchicalID(heirarchicalID string) (float64, error) {
	token := TokenID(heirarchicalID)
	tokenLevel := token.Level()
	tokenValue, err := wallet.LevelToDenom(tokenLevel)
	if err != nil {
		return 0.0, fmt.Errorf(
			"GetTokenValueFromHierarchicalID: failed to get token value for level: %v, token: %v",
			tokenLevel,
			heirarchicalID,
		)
	}

	return tokenValue, nil
}
