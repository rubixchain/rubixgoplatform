package util

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func GetNetworkMode(testnet, mainnet, localnet bool) (string, error) {
	count := 0
	if testnet {
		count++
	}
	if mainnet {
		count++
	}
	if localnet {
		count++
	}

	if count != 1 {
		return "", fmt.Errorf("exactly one network mode must be selected")
	}

	switch {
	case testnet:
		return constants.NetworkMode_Testnet, nil
	case mainnet:
		return constants.NetworkMode_Mainnet, nil
	default:
		return constants.NetworkMode_Localnet, nil
	}
}

func ExtractTokenIDs(tokens []*models.TokenInfo) []string {
	tokenIDs := make([]string, 0, len(tokens)) // pre-allocate capacity

	for _, token := range tokens {
		tokenIDs = append(tokenIDs, token.TokenID)
	}

	return tokenIDs
}
