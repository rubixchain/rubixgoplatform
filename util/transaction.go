package util

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func GetTransactionID(txInfo *models.TransactionInfo) (string, error) {
	txInfoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return "", fmt.Errorf("GetTransactionID: failed to serialize TransactionInfo, err: %v", err)
	}

	txHashBytes := CalculateHash(txInfoBytes, constants.HashAlgorithm_SHA3_256)
	return hex.EncodeToString(txHashBytes), nil
}

func SignTransaction(dc types.DIDCrypto, txInfo *models.TransactionInfo) (string, error) {
	infoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return "", fmt.Errorf("SignTransaction: failed to serialize TransactionInfo, err: %v", err)
	}
	// Hash before signing so the full payload (not just the first 32 bytes) is bound
	// to the signature. ECDSA operates on a fixed-size digest; passing raw variable-length
	// JSON allows an attacker to forge a valid signature for a different payload that
	// shares only the first N bytes (curve-order truncation attack).
	digest := CalculateHash(infoBytes, constants.HashAlgorithm_SHA3_256)
	signatureBytes, err := dc.PvtSign(digest)
	if err != nil {
		return "", fmt.Errorf("SignTransaction: failed to sign transaction info, err: %v", err)
	}
	signature := hex.EncodeToString(signatureBytes)
	return signature, nil
}

func VerifySignature(dc types.DIDCrypto, txInfo *models.TransactionInfo, signature string) error {
	infoBytes, err := models.SerializeTransactionInfo(txInfo)
	if err != nil {
		return fmt.Errorf("VerifySignature: failed to serialize TransactionInfo, err: %w", err)
	}
	// Must hash identically to SignTransaction so the digest is the same 32-byte value.
	digest := CalculateHash(infoBytes, constants.HashAlgorithm_SHA3_256)

	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("VerifySignature: failed to decode hex signature, err: %w", err)
	}

	ok, err := dc.PvtVerify(digest, signatureBytes)
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

	txInfoBytes, err := models.SerializeTransactionInfo(tx)
	if err != nil {
		return nil, fmt.Errorf("PublishTransaction: failed to serialize TransactionInfo, err: %v", err)
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
