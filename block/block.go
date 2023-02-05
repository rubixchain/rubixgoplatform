package block

import (
	"fmt"
	"strconv"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	TokenBlockType int = iota
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
	TCPreviousBlockIDKey   string = "previousBlockID"
	TCBlockHashKey         string = "hash"
	TCNonceKey             string = "nonce"
	TCBlockContentKey      string = "blockContent"
	TCSignatureKey         string = "signature"
	TCSenderSignKey        string = "senderSign"
	TCPvtShareKey          string = "pvtShareBits"
	TCTokenChainBlockKey   string = "tokenChainBlock"
)

type TokenChainBlock struct {
	BlockType         int                    `json:"blkType"`
	TransactionType   string                 `json:"transactionType"`
	TokenID           string                 `json:"tokenID"`
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
	TokensPledgeMap   map[string]interface{} `json:"tokensPledgeMap"`
	TokenChainDetials map[string]interface{} `json:"tokenChainBlock"`
}

type Block struct {
	bt int
	bb []byte
	bm map[string]interface{}
}

func InitBlock(bt int, bb []byte, bm map[string]interface{}) *Block {
	b := &Block{
		bt: bt,
		bb: bb,
		bm: bm,
	}
	if b.bb == nil && b.bm == nil {
		return nil
	}
	var err error
	if b.bb == nil {
		err = b.blkEncode()
		if err != nil {
			return nil
		}
	}
	if b.bm == nil {
		err = b.blkDecode()
		if err != nil {
			return nil
		}
	}
	return b
}

func CreateNewBlock(ctcb map[string]*Block, tcb *TokenChainBlock) *Block {
	ntcb := make(map[string]interface{})
	ntcb[TCTransTypeKey] = tcb.TransactionType
	ntcb[TCOwnerKey] = tcb.TokenOwner
	ntcb[TCCommentKey] = tcb.Comment
	if tcb.SenderDID != "" {
		ntcb[TCSenderDIDKey] = tcb.SenderDID
	}
	if tcb.ReceiverDID != "" {
		ntcb[TCReceiverDIDKey] = tcb.ReceiverDID
	}
	if tcb.TID != "" {
		ntcb[TCTIDKey] = tcb.TID
	}
	if len(tcb.WholeTokens) != 0 {
		ntcb[TCWholeTokensKey] = tcb.WholeTokens
	}
	if len(tcb.WholeTokensID) != 0 {
		ntcb[TCWholeTokensIDKey] = tcb.WholeTokensID
	}
	if len(tcb.PartTokens) != 0 {
		ntcb[TCPartTokensKey] = tcb.PartTokens
	}
	if len(tcb.PartTokensID) != 0 {
		ntcb[TCPartTokensIDKey] = tcb.PartTokensID
	}
	if tcb.QuorumSignature != nil {
		ntcb[TCQuorumSignatureKey] = tcb.QuorumSignature
	}
	if len(tcb.TokensPledgedFor) != 0 {
		ntcb[TCTokensPledgedForKey] = tcb.TokensPledgedFor
	}
	if len(tcb.TokensPledgedWith) != 0 {
		ntcb[TCTokensPledgedWithKey] = tcb.TokensPledgedWith
	}
	if tcb.TokensPledgeMap != nil {
		ntcb[TCTokensPledgeMapKey] = tcb.TokensPledgeMap
	}
	if tcb.TokenChainDetials != nil {
		ntcb[TCTokenChainBlockKey] = tcb.TokenChainDetials
	}
	if ctcb == nil {
		return nil
	}
	phm := make(map[interface{}]interface{}, 0)
	bnm := make(map[interface{}]interface{}, 0)
	for t, b := range ctcb {
		if b == nil {
			bnm[t] = "0"
			phm[t] = ""
		} else {
			bn, err := b.GetBlockNumber(t)
			if err != nil {
				return nil
			}
			bn++
			bid, err := b.GetBlockID(t)
			if err != nil {
				return nil
			}
			phm[t] = bid
			bnm[t] = strconv.FormatUint(bn, 10)
		}
	}
	ntcb[TCBlockNumber] = bnm
	ntcb[TCPreviousBlockIDKey] = phm
	blk := InitBlock(tcb.BlockType, nil, ntcb)
	return blk
}

func (b *Block) blkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(b.bb, &m)
	if err != nil {
		return nil
	}
	si, ok := m[TCSignatureKey]
	if !ok {
		return fmt.Errorf("invalid block, missing signature")
	}
	bc, ok := m[TCBlockContentKey]
	if !ok {
		return fmt.Errorf("invalid block, missing block content")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	var tcb map[string]interface{}
	err = cbor.Unmarshal(bc.([]byte), &tcb)
	if err != nil {
		return err
	}
	tcb[TCBlockHashKey] = util.HexToStr(hb)
	tcb[TCSignatureKey] = util.HexToStr(si.([]byte))
	b.bm = tcb
	return nil
}

func (b *Block) blkEncode() error {
	// Remove Hash & Signature before CBOR conversation
	_, hok := b.bm[TCBlockHashKey]
	if hok {
		delete(b.bm, TCBlockHashKey)
	}
	s, sok := b.bm[TCSignatureKey]
	if sok {
		delete(b.bm, TCSignatureKey)
	}
	bc, err := cbor.Marshal(b.bm, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	hb := util.CalculateHash(bc, "SHA3-256")
	b.bm[TCBlockHashKey] = util.HexToStr(hb)
	m := make(map[string]interface{})
	m[TCBlockContentKey] = bc
	if sok {
		b.bm[TCSignatureKey] = s
		m[TCSignatureKey] = util.StrToHex(s.(string))
	}
	blk, err := cbor.Marshal(m, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	b.bb = blk
	return nil
}

func (b *Block) GetBlockNumber(t string) (uint64, error) {
	bnmi, ok := b.bm[TCBlockNumber]
	if !ok {
		return 0, fmt.Errorf("invalid token chain block, missing block number")
	}
	bnm := bnmi.(map[interface{}]interface{})
	si, ok := bnm[t]
	if !ok {
		return 0, fmt.Errorf("invalid token chain block, missing block number")
	}
	num, err := strconv.ParseUint(si.(string), 10, 64)
	if err != nil {
		return 0, err
	}
	return num, nil
}

func (b *Block) GetBlockID(t string) (string, error) {
	ha, ok := b.bm[TCBlockHashKey]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	bnmi, ok := b.bm[TCBlockNumber]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	bnm := bnmi.(map[interface{}]interface{})
	si, ok := bnm[t]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	return si.(string) + "-" + ha.(string), nil
}

func (b *Block) GetPrevBlockID(t string) (string, error) {
	phmi, ok := b.bm[TCPreviousBlockIDKey]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	phm := phmi.(map[interface{}]interface{})
	bid, ok := phm[t]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	return bid.(string), nil
}

func (b *Block) GetHashSig() (string, string, error) {
	h, ok := b.bm[TCBlockHashKey]
	if !ok {
		return "", "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	s, ok := b.bm[TCSignatureKey]
	if !ok {
		return "", "", fmt.Errorf("invalid token chain block, missing block signature")
	}
	return h.(string), s.(string), nil
}

func (b *Block) UpdateSignature(sig string) error {
	b.bm[TCSignatureKey] = sig
	return b.blkEncode()
}

func (b *Block) GetBlock() []byte {
	return b.bb
}

func (b *Block) GetBlockMap() map[string]interface{} {
	return b.bm
}

func (b *Block) GetHash() (string, error) {
	h, ok := b.bm[TCBlockHashKey]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	return h.(string), nil
}

func (b *Block) GetTransType() string {
	tt, ok := b.bm[TCTransTypeKey]
	if !ok {
		return ""
	}
	return tt.(string)
}

func (b *Block) GetSenderDID() string {
	tt, ok := b.bm[TCSenderDIDKey]
	if !ok {
		return ""
	}
	return tt.(string)
}

func (b *Block) GetReceiverDID() string {
	tt, ok := b.bm[TCReceiverDIDKey]
	if !ok {
		return ""
	}
	return tt.(string)
}
