package util

import (
	"encoding/json"
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

func UnzipMap[K comparable, V any](m map[K]V) ([]K, []V) {
	keys := make([]K, 0, len(m))
	values := make([]V, 0, len(m))

	for k, v := range m {
		keys = append(keys, k)
		values = append(values, v)
	}

	return keys, values
}

// ConvertToTokenChainResponses converts an array of Transactions into TokenChainResponse array.
// It parses the JSON Info field to extract Initiator, Epoch, and Data fields.
// It works for both SmartContract and NFT token types, extracting data from whichever is present.
func ConvertToTokenChainResponses(transactions []models.Transactions) ([]models.TokenChainResponse, error) {
	responses := make([]models.TokenChainResponse, 0, len(transactions))

	for _, txn := range transactions {
		// Parse the transaction info JSON
		var txInfo models.TransactionInfo
		if err := json.Unmarshal(txn.Info, &txInfo); err != nil {
			return nil, fmt.Errorf("convertToTokenChainResponses: failed to unmarshal transaction info for txn %s: %w", txn.ID, err)
		}

		// Extract Data from SmartContract or NFT tokens
		var data string
		if txInfo.Tokens != nil {
			// Check SmartContract tokens first
			if len(txInfo.Tokens.SmartContract) > 0 {
				data = txInfo.Tokens.SmartContract[0].Data
			} else if len(txInfo.Tokens.NFT) > 0 {
				// Fall back to NFT tokens if no SmartContract
				data = txInfo.Tokens.NFT[0].Data
			}
		}

		// Create the response object
		response := models.TokenChainResponse{
			TransactionID: txn.ID,
			Initiator:     txInfo.Initiator,
			Epoch:         txInfo.Epoch,
			Data:          data,
		}

		responses = append(responses, response)
	}

	return responses, nil
}
