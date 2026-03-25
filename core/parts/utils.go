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

func ValidationOfTokenValue(tokenID string, assetType int, tokenValue float64) bool {
	if assetType == RBTTokenType {
		computedValue, err := util.GetTokenValueFromTokenID(tokenID)
		if err != nil {
			return false
		}

		return computedValue == tokenValue
	}
	if assetType == FTTokenType {
		//TODO: Get the token value from the genesis transaction info from the tokenchain table of the tokenID
	}
	if assetType == NFTTokenType || assetType == SmartContractTokenType {
		//TODO: Get the token value from the json file by doing ipfs cat operation on the tokenID
	}
	return false
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
