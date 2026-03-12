package parts

import (
	"fmt"
	"math"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)


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

// GetTokenValueFromIndexedID fetches the token value by
// looking at the IPFS content
func GetTokenValueFromIndexedID(indexedID string) (float64, error) {
	hierarchicalID, err := IndexedToHierarchical(indexedID)
	if err != nil {
		return rubixmath.ZeroFloat(), fmt.Errorf("GetTokenValueFromIndexedID: failed to convert indexed ID to hierarchical ID, err: %v", err)
	}

	return GetTokenValueFromHierarchicalID(string(hierarchicalID))
}

// GetTokenValueFromHierarchicalID fetches the token value by
// looking at its hierarchical structure.
//
// Make sure to pass the IPFS content, and not the IPFS ID of the token
func GetTokenValueFromHierarchicalID(heirarchicalID string) (float64, error) {
	token := TokenID(heirarchicalID)
	tokenLevel := token.Level()
	tokenValue, err := util.LevelToDenom(tokenLevel)
	if err != nil {
		return 0.0, fmt.Errorf(
			"GetTokenValueFromHierarchicalID: failed to get token value for level: %v, token: %v",
			tokenLevel,
			heirarchicalID,
		)
	}

	return tokenValue, nil
}

func removeTokensFromList(tokens []models.Token, tokenIDsToRemove []models.Token) []models.Token {
	if len(tokenIDsToRemove) == 0 {
		return tokens
	}

	tokenIDSet := make(map[string]struct{})
	for _, token := range tokenIDsToRemove {
		tokenIDSet[token.TokenID] = struct{}{}
	}

	var filteredTokens []models.Token
	for _, token := range tokens {
		if _, found := tokenIDSet[token.TokenID]; !found {
			filteredTokens = append(filteredTokens, token)
		}
	}

	return filteredTokens
}
