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
	TCBlockNumber          string = "blockNumber"
	TCOwnerKey             string = "owner"
	TCSenderDIDKey         string = "sender"
	TCReceiverDIDKey       string = "receiver"
	TCCommentKey           string = "comment"
	TCTIDKey               string = "tid"
	TCGroupKey             string = "group"
	TCWholeTokensKey       string = "wholeTokens"
	TCWholeTokensIDKey     string = "wholeTokensID"
	TCPartTokensKey        string = "partTokens"
	TCPartTokensIDKey      string = "partTokensID"
	TCQuorumSignatureKey   string = "quorumSignature"
	TCPledgeTokenKey       string = "pledgeToken"
	TCTokensPledgedForKey  string = "tokensPledgedFor"
	TCTokensPledgedWithKey string = "tokensPledgedWith"
	TCTokensPledgeMapKey   string = "tokensPledgeMap"
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
	TransactionType   string                 `json:"transactionType"`
	TokenOwner        string                 `json:"owner"`
	SenderDID         string                 `json:"sender"`
	ReceiverDID       string                 `json:"receiver"`
	Comment           string                 `json:"comment"`
	TID               string                 `json:"tid"`
	WholeTokens       []string               `json:"wholeTokens"`
	WholeTokensID     []string               `json:"wholeTokensID"`
	PartTokens        []string               `json:"partTokens"`
	PartTokensID      []string               `json:"partTokensID"`
	QuorumSignature   []string               `json:"quorumSignature"`
	TokensPledgedFor  []string               `json:"tokensPledgedFor"`
	TokensPledgedWith []string               `json:"tokensPledgedWith"`
	TokensPledgeMap   []string               `json:"tokensPledgeMap"`
	TokenChainDetials map[string]interface{} `json:"tokenChainBlock"`
}

type TokenChainStatus struct {
	TransactionType string `json:"transactionType"`
	BlockID         string `json:"blockID"`
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
	err = w.ts.Read(token, &kv, "key=?", ts.BlockID)
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
	bid, err := GetBlockID(token, tcb)
	if err != nil {
		return err
	}
	kv.Key = bid
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
	bid, err := GetBlockID(token, tcb)
	if err != nil {
		return err
	}
	owner, ok := tcb[TCOwnerKey]
	if !ok {
		return fmt.Errorf("invalid token chain block")
	}
	ts := TokenChainStatus{
		TransactionType: transType.(string),
		BlockID:         bid,
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
	kv.Key = bid
	kv.Value = string(jb)
	return w.ts.Write(token, &kv)
}
