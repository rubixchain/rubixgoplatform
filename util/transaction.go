package util

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

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

// TransactionToBytes converts transaction struct into a byte array
func TransactionToBytes(txn *models.Transactions) ([]byte, error) {
	var buf bytes.Buffer

	// Write ID (prefixed with its length as uint32)
	idBytes := []byte(txn.ID)
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(idBytes))); err != nil {
		return nil, err
	}
	buf.Write(idBytes)

	// Write Info (prefixed with its length as uint32)
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(txn.Info))); err != nil {
		return nil, err
	}
	buf.Write(txn.Info)

	// Write Signature (prefixed with its length as uint32)
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(txn.Signature))); err != nil {
		return nil, err
	}
	buf.Write(txn.Signature)

	return buf.Bytes(), nil
}

func TransactionFromBytes(data []byte) (*models.Transactions, error) {
	buf := bytes.NewReader(data)
	t := &models.Transactions{}

	// Read ID
	var idLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &idLen); err != nil {
		return nil, err
	}
	idBytes := make([]byte, idLen)
	if _, err := io.ReadFull(buf, idBytes); err != nil {
		return nil, err
	}
	t.ID = string(idBytes)

	// Read Info
	var infoLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &infoLen); err != nil {
		return nil, err
	}
	t.Info = make(json.RawMessage, infoLen)
	if _, err := io.ReadFull(buf, t.Info); err != nil {
		return nil, err
	}

	// Read Signature
	var sigLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &sigLen); err != nil {
		return nil, err
	}
	t.Signature = make(json.RawMessage, sigLen)
	if _, err := io.ReadFull(buf, t.Signature); err != nil {
		return nil, err
	}

	return t, nil
}
