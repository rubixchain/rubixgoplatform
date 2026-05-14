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

// ConvertToTokenChainResponses converts raw transaction rows into the per-token
// chain response. targetTokenID is the token whose chain is being requested; the
// Data field is taken from the matching NFT or SmartContract entry inside each
// transaction, not blindly from index 0. This matters for transactions that
// carry multiple NFT entries (e.g. child-mint: parent execute + child deploy).
func ConvertToTokenChainResponses(transactions []models.Transactions, targetTokenID string) ([]models.TokenChainResponse, error) {
	responses := make([]models.TokenChainResponse, 0, len(transactions))

	for _, txn := range transactions {
		// Parse the transaction info JSON
		var txInfo models.TransactionInfo
		if err := json.Unmarshal(txn.Info, &txInfo); err != nil {
			return nil, fmt.Errorf("convertToTokenChainResponses: failed to unmarshal transaction info for txn %s: %w", txn.ID, err)
		}

		// Pick the data field from the SmartContract or NFT entry that matches
		// targetTokenID. Falls back to "" when no entry matches.
		var data string
		if txInfo.Tokens != nil {
			for _, sc := range txInfo.Tokens.SmartContract {
				if sc != nil && sc.TokenID == targetTokenID {
					data = sc.Data
					break
				}
			}
			if data == "" {
				for _, n := range txInfo.Tokens.NFT {
					if n != nil && n.TokenID == targetTokenID {
						data = n.Data
						break
					}
				}
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

func ExtractResult[T any](result interface{}) (T, error) {
	var target T
	b, err := json.Marshal(result)
	if err != nil {
		return target, fmt.Errorf("failed to marshal result: %w", err)
	}
	if err := json.Unmarshal(b, &target); err != nil {
		return target, fmt.Errorf("failed to unmarshal result: %w", err)
	}
	return target, nil
}
