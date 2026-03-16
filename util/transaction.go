package util

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func GetTransactionID(txInfo *models.TransactionInfo) (string, error) {
	txInfoBytes, err := json.Marshal(txInfo)
	if err != nil {
		return "", fmt.Errorf("GetTransactionID: failed to marshal TransactionInfo, err: %v", er)
	}

	txHashBytes := CalculateHash(txInfoBytes, constants.HashAlgorithm_SHA3_256)	
	return string(txHashBytes), nil
}