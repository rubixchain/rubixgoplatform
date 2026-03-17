package util

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func GetTransactionID(txInfo *models.TransactionInfo) (string, error) {
	txInfoBytes, err := json.Marshal(txInfo)
	if err != nil {
		return "", fmt.Errorf("GetTransactionID: failed to marshal TransactionInfo, err: %v", err)
	}

	txHashBytes := CalculateHash(txInfoBytes, constants.HashAlgorithm_SHA3_256)
	return string(txHashBytes), nil
}

func SignTransaction(dc types.DIDCrypto, txInfo *models.TransactionInfo) (string, error) {
	transactionId, err := GetTransactionID(txInfo)
	if err != nil {
		return "", fmt.Errorf("SignTransaction: failed to get transaction ID, err: %v", err)
	}
	signatureBytes, err := dc.PvtSign([]byte(transactionId))
	if err != nil {
		return "", fmt.Errorf("SignTransaction: failed to sign transaction ID, err: %v", err)
	}
	signature := base64.StdEncoding.EncodeToString(signatureBytes)
	return signature, nil
}

func VerifySignature(dc types.DIDCrypto, txInfo *models.TransactionInfo, signature string) error {
	transactionId, err := GetTransactionID(txInfo)
	if err != nil {
		return fmt.Errorf("VerifySignature: failed to get transaction ID, err: %w", err)
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("VerifySignature: failed to decode signature, err: %w", err)
	}

	ok, err := dc.PvtVerify([]byte(transactionId), signatureBytes)
	if err != nil {
		return fmt.Errorf("VerifySignature: failed to verify signature, err: %w", err)
	}
	if !ok {
		return fmt.Errorf("VerifySignature: signature verification returned false")
	}

	return nil
}

func PublishTransaction(pubsub *types.PubSub, tx *models.TransactionInfo, signature *models.Signature) (*models.Transactions, error) {
	txID, err := GetTransactionID(tx)
	if err != nil {
		return nil, fmt.Errorf("PublishTransaction: failed to get transaction ID: %v", err)
	}

	txInfoBytes, err := json.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("PublishTransaction: failed to marshal transactionInfo, err: %v", err)
	}

	signatureBytes, err := json.Marshal(signature)
	if err != nil {
		return nil, fmt.Errorf("PublishTransaction: failed to marshal Signature, err: %v", err)
	}

	transaction := &models.Transactions{
		ID:        txID,
		Info:      txInfoBytes,
		Signature: signatureBytes,
	}

	eventTx := models.EventTransaction{
		Status:      true,
		Transaction: transaction,
	}

	if err := pubsub.Publish(constants.Event_RubixTxns, eventTx); err != nil {
		return nil, fmt.Errorf("PublishTransaction: failed to publish genesis transaction, err: %v", err)
	}

	return transaction, nil
}
