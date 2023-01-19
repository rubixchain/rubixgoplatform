package wallet

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	TokenStatusStorage string = "TokenStatus"
)

const (
	TokenMintedType      string = "token_minted"
	TokenTransferredType string = "token_transferred"
	TokenMigratedType    string = "token_migrated"
	TokenPledgedType     string = "token_pledged"
	TokenGeneratedType   string = "token_generated"
)

const (
	TCTransTypeKey         string = "transactionType"
	TCOwnerKey             string = "owner"
	TCTokenIDKey           string = "tokenID"
	TCSenderDIDKey         string = "sender"
	TCReceiverDIDKey       string = "receiver"
	TCCommentKey           string = "comment"
	TCTIDKey               string = "tid"
	TCGroupKey             string = "group"
	TCPledgeTokenKey       string = "pledgeToken"
	TCTokensPledgedForKey  string = "tokensPledgedFor"
	TCTokensPledgedWithKey string = "tokensPledgedWith"
	TCDistributedObjectKey string = "distributedObject"
	TCPreviousHashKey      string = "previousHash"
	TCBlockHashKey         string = "hash"
	TCNonceKey             string = "nonce"
	TCSignatureKey         string = "signature"
	TCSenderSignKey        string = "senderSign"
	TCPvtShareKey          string = "pvtShareBits"
	TCTokenChainBlockKey   string = "tokenChainBlock"
)

type TokenChainBlock struct {
	TransactionType   string            `json:"transactionType"`
	TokenID           string            `json:"token_id"`
	SenderDID         string            `json:"sender"`
	ReceiverDID       string            `json:"receiver"`
	Comment           string            `json:"comment"`
	TID               string            `json:"tid"`
	Group             []string          `json:"group"`
	PledgeToken       string            `json:"pledgeToken"`
	TokensPledgedFor  []string          `json:"tokensPledgedFor"`
	TokensPledgedWith []string          `json:"tokensPledgedWith"`
	DistributedObject map[string]string `json:"distributedObject"`
	PreviousHash      string            `json:"previousHash"`
	BlockHash         string            `json:"blockHash"`
	Nonce             string            `json:"nonce"`
	Signature         string            `json:"signature"`
}

type TokenChainStatus struct {
	TransactionType string `json:"transactionType"`
	BlockHash       string `json:"blockHash"`
	TokenOwner      string `json:"owner"`
}

// GetTokenBlock get token chain block from the storage
func (w *Wallet) GetTokenBlock(token string, blockHash string) (map[string]interface{}, error) {
	var kv storage.StorageType
	err := w.ts.Read(token, &kv, "key=?", blockHash)
	if err != nil {
		return nil, err
	}
	tcb := make(map[string]interface{})
	err = json.Unmarshal([]byte(kv.Value), &tcb)
	if err != nil {
		return nil, fmt.Errorf("Invalid token chain block, " + err.Error())
	}
	return tcb, nil
}

// GetLatestTokenBlock get latest token block from the storage
func (w *Wallet) GetLatestTokenBlock(token string) (map[string]interface{}, error) {
	var kv storage.StorageType
	err := w.ts.Read(TokenStatusStorage, &kv, "key=?", token)
	if err != nil {
		return nil, err
	}
	var ts TokenChainStatus
	err = json.Unmarshal([]byte(kv.Value), &ts)
	if err != nil {
		return nil, fmt.Errorf("Invalid token chain block, " + err.Error())
	}
	err = w.ts.Read(token, &kv, "key=?", ts.BlockHash)
	if err != nil {
		return nil, err
	}
	tcb := make(map[string]interface{})
	err = json.Unmarshal([]byte(kv.Value), &tcb)
	if err != nil {
		return nil, fmt.Errorf("Invalid token chain block, " + err.Error())
	}
	return tcb, nil
}

// AddTokenBlock will write token block into storage
func (w *Wallet) AddTokenBlock(token string, tcb map[string]interface{}) error {
	var kv storage.StorageType
	hash, ok := tcb[TCBlockHashKey]
	if !ok {
		return fmt.Errorf("block hash is missing")
	}
	kv.Key = hash.(string)
	jb, err := json.Marshal(tcb)
	if err != nil {
		fmt.Errorf("Failed to marshal json, " + err.Error())
	}
	kv.Value = string(jb)
	return w.ts.Write(token, &kv)
}

// AddLatestTokenBlock will write token block into storage
func (w *Wallet) AddLatestTokenBlock(token string, tcb map[string]interface{}) error {
	var kv storage.StorageType
	transType, ok := tcb[TCTransTypeKey]
	if !ok {
		return fmt.Errorf("invalid token chain block")
	}
	hash, ok := tcb[TCBlockHashKey]
	if !ok {
		return fmt.Errorf("invalid token chain block")
	}
	owner, ok := tcb[TCOwnerKey]
	if !ok {
		return fmt.Errorf("invalid token chain block")
	}
	ts := TokenChainStatus{
		TransactionType: transType.(string),
		BlockHash:       hash.(string),
		TokenOwner:      owner.(string),
	}
	tsb, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	kv.Key = token
	kv.Value = string(tsb)
	err = w.ts.Write(TokenStatusStorage, &kv)
	if err != nil {
		return err
	}

	jb, err := json.Marshal(tcb)
	if err != nil {
		return err
	}
	kv.Key = hash.(string)
	kv.Value = string(jb)
	return w.ts.Write(token, &kv)
}
