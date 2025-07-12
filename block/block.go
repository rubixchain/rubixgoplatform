package block

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/fxamacker/cbor"
	didmodule "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ----------TokenChain----------------------
// {
// 	 "1" : TokenType        : int
// 	 "2" : TransactionType  : string
// 	 "3" : TokenOwner       : string
// 	 "4" : GenesisBlock     : GenesisBlock
//   "5" : TransInfo        : TransInfo
//   "6" : SmartContract    : []byte
//   "7" : QuorumSignature  : []string
//   "8" : PledgeDetails    : map[string][]PledgeDetail
//   "9" : SmartContractData : string
//
// }

const (
	TCTokenTypeKey          string = "1"
	TCTransTypeKey          string = "2"
	TCTokenOwnerKey         string = "3"
	TCGenesisBlockKey       string = "4"
	TCTransInfoKey          string = "5"
	TCSmartContractKey      string = "6"
	TCQuorumSignatureKey    string = "7"
	TCPledgeDetailsKey      string = "8"
	TCBlockHashKey          string = "98"
	TCSignatureKey          string = "99"
	TCBlockContentKey       string = "1"
	TCBlockContentSigKey    string = "2"
	TCSmartContractDataKey  string = "9"
	TCTokenValueKey         string = "10"
	TCChildTokensKey        string = "11"
	TCInitiatorSignatureKey string = "12"
	TCEpochKey              string = "epoch"
	TCNFTDataKey            string = "13"
)

const (
	TokenMintedType               string = "01"
	TokenTransferredType          string = "02"
	TokenMigratedType             string = "03"
	TokenPledgedType              string = "04"
	TokenGeneratedType            string = "05"
	TokenUnpledgedType            string = "06"
	TokenCommittedType            string = "07"
	TokenBurntType                string = "08"
	TokenDeployedType             string = "09"
	TokenExecutedType             string = "10"
	TokenContractCommited         string = "11"
	TokenPinnedAsService          string = "12"
	TokenIsBurntForFT             string = "13"
	SpendableTokenTransferredType string = "14" // cvr-1 block
	OwnershipTransferredType      string = "15" // cvr-2 block
)

const (
	InitiatorNLSSShare   string = "nlss_share_signature"
	InitiatorPrivateSign string = "priv_signature"
	InitiatorDID         string = "InitiatorDID"
	InitiatorHash        string = "hash"
	InitiatorSignType    string = "sign_type"
)

const (
	CreditSigSignature     string = "signature"
	CreditSigPrivSignature string = "priv_signature"
	CreditSigDID           string = "did"
	CreditSigHash          string = "hash"
	CreditSigSignType      string = "sign_type"
)

type TokenChainBlock struct {
	TransactionType    string              `json:"transactionType"`
	TokenOwner         string              `json:"owner"`
	GenesisBlock       *GenesisBlock       `json:"genesisBlock"`
	TransInfo          *TransInfo          `json:"transInfo"`
	PledgeDetails      []PledgeDetail      `json:"pledgeDetails"`
	QuorumSignature    []CreditSignature   `json:"quorumSignature"`
	SmartContract      []byte              `json:"smartContract"`
	SmartContractData  string              `json:"smartContractData"`
	TokenValue         float64             `json:"tokenValue"`
	ChildTokens        []string            `json:"childTokens"`
	InitiatorSignature *InitiatorSignature `json:"initiatorSignature"`
	NFT                []byte              `json:"nft"`
	NFTData            string              `json:"nftData"`
	Epoch              int                 `json:"epoch"`
}

type PledgeDetail struct {
	Token        string `json:"token"`
	TokenType    int    `json:"tokenType"`
	DID          string `json:"did"`
	TokenBlockID string `json:"tokenBlockID"`
}

type Block struct {
	bb  []byte
	bm  map[string]interface{}
	op  bool
	log logger.Logger
}

type CreditSignature struct {
	Signature     string `json:"signature"`
	PrivSignature string `json:"priv_signature"`
	DID           string `json:"did"`
	Hash          string `json:"hash"`
	SignType      string `json:"sign_type"` //represents sign type (PkiSign == 0 or NlssSign==1)
}

type InitiatorSignature struct {
	NLSSShare   string `json:"nlss_share_signature"`
	PrivateSign string `json:"priv_signature"`
	DID         string `json:"InitiatorDID"`
	Hash        string `json:"hash"`
	SignType    int    `json:"sign_type"` //represents sign type (PkiSign == 0 or NlssSign==1)
}

type BlockOption func(b *Block)

func NoSignature() BlockOption {
	// this is the ClientOption function type
	return func(b *Block) {
		b.op = true
	}
}

func InitBlock(bb []byte, bm map[string]interface{}, opts ...BlockOption) *Block {
	b := &Block{
		bb: bb,
		bm: bm,
		op: false,
	}
	if b.bb == nil && b.bm == nil {
		fmt.Println("getting empty map and bytes")
		return nil
	}
	for _, opt := range opts {
		opt(b)
	}
	var err error
	if b.bb == nil {
		err = b.blkEncode()
		if err != nil {
			fmt.Println("getting empty bytes 1, err ", err)
			return nil
		}
	}
	if b.bm == nil {
		err = b.blkDecode()
		if err != nil {
			fmt.Println("getting empty map 2, err ", err)
			return nil
		}
	}
	return b
}

func CreateNewBlock(ctcb map[string]*Block, tcb *TokenChainBlock) *Block {
	if tcb.TransInfo == nil || ctcb == nil {
		fmt.Println("empty tokens and their latest block")
		return nil
	}
	ntcb := make(map[string]interface{})
	ntcb[TCTransTypeKey] = tcb.TransactionType
	ntcb[TCTokenOwnerKey] = tcb.TokenOwner
	if tcb.GenesisBlock != nil {
		ntcb[TCGenesisBlockKey] = newGenesisBlock(tcb.GenesisBlock)
		if ntcb[TCGenesisBlockKey] == nil {
			fmt.Println("failed to initiate genesis block")
			return nil
		}
	}
	ntib := newTransInfo(ctcb, tcb.TransInfo)
	if ntib == nil {
		fmt.Println("failed to initiate trans-info")
		return nil
	}
	ntcb[TCTransInfoKey] = ntib
	pdib := newPledgeDetails(tcb.PledgeDetails)
	if pdib != nil {
		ntcb[TCPledgeDetailsKey] = pdib
	}
	if tcb.QuorumSignature != nil {
		ntcb[TCQuorumSignatureKey] = tcb.QuorumSignature
	}
	if tcb.SmartContract != nil {
		ntcb[TCSmartContractKey] = tcb.SmartContract
	}
	if tcb.SmartContractData != "" {
		ntcb[TCSmartContractDataKey] = tcb.SmartContractData
	}
	if tcb.NFTData != "" {
		ntcb[TCNFTDataKey] = tcb.NFTData
	}
	if tcb.InitiatorSignature != nil {
		ntcb[TCInitiatorSignatureKey] = tcb.InitiatorSignature
	}

	if floatPrecisionToMaxDecimalPlaces(tcb.TokenValue) > floatPrecisionToMaxDecimalPlaces(0) {
		ntcb[TCTokenValueKey] = floatPrecisionToMaxDecimalPlaces(tcb.TokenValue)
	}

	if len(tcb.ChildTokens) == 0 {
		ntcb[TCChildTokensKey] = []string{}
	} else {
		ntcb[TCChildTokensKey] = tcb.ChildTokens
	}

	if tcb.Epoch != 0 {
		ntcb[TCEpochKey] = tcb.Epoch
	}

	blk := InitBlock(nil, ntcb)
	return blk
}

func (b *Block) blkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(b.bb, &m)
	if err != nil {
		fmt.Println("failed to decode block", err.Error(), err)
		return nil
	}
	si, sok := m[TCBlockContentSigKey]
	if !sok && !b.op {
		return fmt.Errorf("invalid block, missing signature")
	}
	initiatorSig, iok := m[TCInitiatorSignatureKey]
	if iok {
		delete(b.bm, TCInitiatorSignatureKey)
	}
	quorumSig, qok := m[TCQuorumSignatureKey]
	if qok {
		delete(b.bm, TCQuorumSignatureKey)
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
	if sok {
		var ksb map[string]interface{}
		err = cbor.Unmarshal(si.([]byte), &ksb)
		if err != nil {
			return err
		}
		tcb[TCSignatureKey] = ksb
	}
	if iok {
		tcb[TCInitiatorSignatureKey] = initiatorSig
	}
	if qok {
		tcb[TCQuorumSignatureKey] = quorumSig
	}

	tcb[TCBlockHashKey] = util.HexToStr(hb)

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
	initiatorSig, iok := b.bm[TCInitiatorSignatureKey]
	if iok {
		delete(b.bm, TCInitiatorSignatureKey)
	}
	quorumSig, qok := b.bm[TCQuorumSignatureKey]
	if qok {
		delete(b.bm, TCQuorumSignatureKey)
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
		ksm, err := cbor.Marshal(s, cbor.CanonicalEncOptions())
		if err != nil {
			return err
		}
		m[TCBlockContentSigKey] = ksm
	}
	if iok {
		b.bm[TCInitiatorSignatureKey] = initiatorSig
		m[TCInitiatorSignatureKey] = initiatorSig
	}
	if qok {
		b.bm[TCQuorumSignatureKey] = quorumSig
		m[TCQuorumSignatureKey] = quorumSig
	}
	blk, err := cbor.Marshal(m, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	b.bb = blk
	return nil
}

func (b *Block) getTokensMap(t string) interface{} {
	if b.bm == nil {
		fmt.Println("b.bm is empty")
		return nil
	}
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return nil
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return nil
	}
	ttm := util.GetFromMap(tm, t)
	return ttm
}

func (b *Block) getGenesisTokenMap(t string) interface{} {
	gbm := util.GetFromMap(b.bm, TCGenesisBlockKey)
	if gbm == nil {
		return nil
	}
	im := util.GetFromMap(gbm, GBInfoKey)
	if im == nil {
		return nil
	}
	gtm := util.GetFromMap(im, t)
	return gtm
}

func (b *Block) GetBlockNumber(t string) (uint64, error) {
	ttm := b.getTokensMap(t)
	if ttm == nil {
		return 0, fmt.Errorf("invalid token chain block, missing transaction token block")
	}
	bni := util.GetFromMap(ttm, TTBlockNumberKey)
	if bni == nil {
		return 0, fmt.Errorf("invalid token chain block, missing block number")
	}
	num, err := strconv.ParseUint(util.GetString(bni), 10, 64)
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
	ttm := b.getTokensMap(t)
	if ttm == nil {
		return "", fmt.Errorf("invalid token chain block, missing transaction token block")
	}
	bni := util.GetFromMap(ttm, TTBlockNumberKey)
	if bni == nil {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	bns := util.GetString(bni)
	if bni == "" {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	return bns + "-" + ha.(string), nil
}

func (b *Block) GetPrevBlockID(t string) (string, error) {
	ttm := b.getTokensMap(t)
	if ttm == nil {
		return "", fmt.Errorf("invalid token chain block, missing transaction token block")
	}
	pbi := util.GetFromMap(ttm, TTPreviousBlockIDKey)
	if pbi == nil {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	return util.GetString(pbi), nil
}

func (b *Block) GetSigner() ([]string, error) {
	ksmi, ok := b.bm[TCSignatureKey]
	if !ok {
		return nil, fmt.Errorf("invalid token chain block, missing block signature")
	}
	ksm, ok := ksmi.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid token chain block, missing block signature")
	}
	did := make([]string, 0)
	for k, _ := range ksm {
		did = append(did, k)
	}
	if len(did) == 0 {
		return nil, fmt.Errorf("invalid token chain block, missing block signature")
	}
	return did, nil
}

func (b *Block) GetHashSig(did string) (string, string, error) {
	h, ok := b.bm[TCBlockHashKey]

	if !ok {
		return "", "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	s, ok := b.bm[TCSignatureKey]
	if !ok {
		return "", "", fmt.Errorf("invalid token chain block, missing block signature")
	}
	ks, ok := s.(map[string]interface{})
	if !ok {
		ks, ok := s.(map[interface{}]interface{})
		if !ok {
			return "", "", fmt.Errorf("invalid signature block")
		}
		ksi, ok := ks[did]
		if !ok {
			return "", "", fmt.Errorf("invalid signature block")
		}
		return h.(string), ksi.(string), nil
	}
	ksi, ok := ks[did]
	if !ok {
		return "", "", fmt.Errorf("invalid signature block")
	}
	return h.(string), ksi.(string), nil
}

func (b *Block) GetSignature(dc didmodule.DIDCrypto) (string, error) {
	h, err := b.GetHash()
	if err != nil {
		return "", fmt.Errorf("failed to get hash")
	}
	sb, err := dc.PvtSign([]byte(h))
	if err != nil {
		return "", fmt.Errorf("failed to get did signature, " + err.Error())
	}
	return util.HexToStr(sb), nil
}

func (b *Block) VerifySignature(dc didmodule.DIDCrypto) error {
	did := dc.GetDID()
	h, s, err := b.GetHashSig(did)
	if err != nil {
		return fmt.Errorf("failed to read did signature & hash")
	}
	ok, err := dc.PvtVerify([]byte(h), util.StrToHex(s))
	if err != nil || !ok {
		return fmt.Errorf("failed to verify did signature")
	}
	return nil
}

func (b *Block) UpdateSignature(dc didmodule.DIDCrypto) error {
	did := dc.GetDID()
	h, err := b.GetHash()
	if err != nil {
		return fmt.Errorf("failed to get hash")
	}
	sb, err := dc.PvtSign([]byte(h))
	if err != nil {
		return fmt.Errorf("failed to get did signature, " + err.Error())
	}
	sig := util.HexToStr(sb)

	ksmi, ok := b.bm[TCSignatureKey]
	if !ok {
		ksm := make(map[string]interface{})
		ksm[did] = sig
		b.bm[TCSignatureKey] = ksm
		return b.blkEncode()
	}
	ksm, ok := ksmi.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signature block")
	}
	ksm[did] = sig
	b.bm[TCSignatureKey] = ksm
	return b.blkEncode()
}

// ReplaceSignature adds each quorums' signature to the block with the key "99"
func (b *Block) ReplaceSignature(did string, sig string) error {
	ksmi, ok := b.bm[TCSignatureKey]
	if !ok {
		ksm := make(map[string]interface{})
		ksm[did] = sig
		b.bm[TCSignatureKey] = ksm
		return b.blkEncode()
	}
	ksm, ok := ksmi.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid signature block")
	}
	ksm[did] = sig
	b.bm[TCSignatureKey] = ksm
	return b.blkEncode()
}

func (b *Block) GetBlock() []byte {
	return b.bb
}

func (b *Block) GetBlockMap() map[string]interface{} {
	return b.bm
}

func (b *Block) getBlkString(key string) string {
	h, ok := b.bm[key]
	if !ok {
		return ""
	}
	return h.(string)
}

func (b *Block) getBlkInt(key string) int {
	tli, ok := b.bm[key]
	if !ok {
		return 0
	}
	var tl int
	switch mt := tli.(type) {
	case int:
		tl = mt
	case int64:
		tl = int(mt)
	case uint64:
		tl = int(mt)
	default:
		tl = 0
	}
	return tl
}

func (b *Block) GetHash() (string, error) {
	h := b.getBlkString(TCBlockHashKey)
	if h == "" {
		return "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	return h, nil
}

func (b *Block) CheckMultiTokenBlock() bool {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return false
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return false
	}
	m, ok := tm.(map[string]interface{})
	if ok {
		return len(m) > 1
	}
	lm, ok := tm.(map[interface{}]interface{})
	if ok {
		return len(lm) > 1
	}
	return false
}

func (b *Block) GetTransTokens() []string {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return nil
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return nil
	}
	m, ok := tm.(map[string]interface{})
	if ok {
		tkns := make([]string, 0)
		for k, _ := range m {
			tkns = append(tkns, k)
		}
		return tkns
	}
	lm, ok := tm.(map[interface{}]interface{})
	if ok {
		tkns := make([]string, 0)
		for k, _ := range lm {
			tkns = append(tkns, k.(string))
		}
		return tkns
	}
	return nil
}

func (b *Block) GetTokenType(t string) int {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return 0
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return 0
	}
	ti := util.GetFromMap(tm, t)
	if ti == nil {
		return 0
	}
	return util.GetIntFromMap(ti, TTTokenTypeKey)
}

func (b *Block) UpdateTokenType(t string, newTokenType int) (*Block, bool) {
	fmt.Println("****** token is : ", t)
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return nil, false
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return nil, false
	}
	ti := util.GetFromMap(tm, t)
	if ti == nil {
		return nil, false
	}
	switch tokenTypeMap := ti.(type) {
	case map[string]interface{}:
		fmt.Println("***** 1. current token type to be updated :", util.GetIntFromMap(ti, TTTokenTypeKey))
		tokenTypeMap[TTTokenTypeKey] = newTokenType
	case map[interface{}]interface{}:
		fmt.Println("***** 2. current token type to be updated :", util.GetIntFromMap(ti, TTTokenTypeKey))
		tokenTypeMap[TTTokenTypeKey] = newTokenType
	}

	updatedBlock := InitBlock(nil, b.bm)
	if updatedBlock == nil {
		fmt.Println("unable to update token type of token : ", t)
		return nil, false
	} else {
		fmt.Println("*****token type updated :", updatedBlock.GetTokenType(t))
	}

	return updatedBlock, true
}

func (b *Block) GetUnpledgeId(t string) string {
	tim := util.GetFromMap(b.bm, TCTransInfoKey)
	if tim == nil {
		return ""
	}
	tm := util.GetFromMap(tim, TITokensKey)
	if tm == nil {
		return ""
	}
	ti := util.GetFromMap(tm, t)
	if ti == nil {
		return ""
	}
	return util.GetStringFromMap(ti, TTUnpledgedIDKey)
}

func (b *Block) GetTokenPledgedForDetails() string {
	return b.getTrasnInfoString(TIRefIDKey)
}

func (b *Block) GetTransType() string {
	return b.getBlkString(TCTransTypeKey)
}

func (b *Block) GetOwner() string {
	return b.getBlkString(TCTokenOwnerKey)
}

func (b *Block) GetSenderDID() string {
	return b.getTrasnInfoString(TISenderDIDKey)
}

func (b *Block) GetReceiverDID() string {
	return b.getTrasnInfoString(TIReceiverDIDKey)
}
func (b *Block) GetDeployerDID() string {
	return b.getTrasnInfoString(TIDeployerDIDKey)
}
func (b *Block) GetPinningNodeDID() string {
	return b.getTrasnInfoString(TIPinningDIDKey)
}

func (b *Block) GetExecutorDID() string {
	return b.getTrasnInfoString(TIExecutorDIDKey)
}

func (b *Block) GetTid() string {
	return b.getTrasnInfoString(TITIDKey)
}

func (b *Block) GetComment() string {
	return b.getTrasnInfoString(TICommentKey)
}

// TODO: correct the below function name
func (b *Block) GetParentDetials(t string) (string, []string, error) {
	gtm := b.getGenesisTokenMap(t)
	if gtm == nil {
		return "", nil, fmt.Errorf("invalid token chain block, missing genesis block")
	}
	p := util.GetStringFromMap(gtm, GIParentIDKey)
	gp := util.GetStringSliceFromMap(gtm, GIGrandParentIDKey)
	return p, gp, nil
}

func (b *Block) GetTokenDetials(t string) (int, int, error) {
	gtm := b.getGenesisTokenMap(t)
	if gtm == nil {
		return 0, 0, fmt.Errorf("invalid token chain block, missing genesis block")
	}
	tl := util.GetIntFromMap(gtm, GITokenLevelKey)
	tn := util.GetIntFromMap(gtm, GITokenNumberKey)
	return tl, tn, nil
}

func (b *Block) GetSmartContract() []byte {
	ci, ok := b.bm[TCSmartContractKey]
	if !ok {
		return nil
	}
	c, ok := ci.([]byte)
	if !ok {
		return nil
	}
	return c
}

func (b *Block) GetCommitedTokenDetials(t string) ([]string, error) {
	genesisTokenMap := b.getGenesisTokenMap(t)
	if genesisTokenMap == nil {
		return nil, fmt.Errorf("invalid token chain block, missing genesis block")
	}
	commitedTokensMap := util.GetFromMap(genesisTokenMap, GICommitedTokensKey)
	if commitedTokensMap == nil {
		return nil, fmt.Errorf("invalid token chain block, missing commited tokens block")
	}
	m, ok := commitedTokensMap.(map[string]interface{})
	if ok {
		tkns := make([]string, 0)
		for k, _ := range m {
			tkns = append(tkns, k)
		}
		return tkns, nil
	}
	lm, ok := commitedTokensMap.(map[interface{}]interface{})
	if ok {
		tkns := make([]string, 0)
		for k, _ := range lm {
			tkns = append(tkns, k.(string))
		}
		return tkns, nil
	}
	return nil, nil
}

// func (b *Block) GetTokenPledgeMap() map[string]interface{} {
// 	tokenPledge := b.bm[TCTokensPledgeMapKey]
// 	tokenPledgeMap, ok := tokenPledge.(map[interface{}]interface{})
// 	if !ok {
// 		return nil
// 	}
// 	result := make(map[string]interface{})
// 	for k, v := range tokenPledgeMap {
// 		kStr, kOk := k.(string)
// 		if !kOk {
// 			return nil
// 		}
// 		result[kStr] = v
// 	}
// 	return result
// }

func (b *Block) GetSmartContractData() string {
	return b.getBlkString(TCSmartContractDataKey)
}

func (b *Block) GetNFTData() string {
	return b.getBlkString(TCNFTDataKey)
}

func (b *Block) GetSmartContractValue(t string) (float64, error) {
	var result float64
	gtm := b.getGenesisTokenMap(t)
	if gtm == nil {
		return result, fmt.Errorf("invalid token chain block, missing genesis block")
	}
	result = util.GetFloatFromMap(gtm, GISmartContractValueKey)
	return result, nil
}

func (b *Block) GetTokenValue() float64 {
	tokenValue := util.GetFloatFromMap(b.bm, TCTokenValueKey)
	return floatPrecisionToMaxDecimalPlaces(tokenValue)
}

func (b *Block) GetChildTokens() []string {
	return util.GetStringSliceFromMap(b.bm, TCChildTokensKey)
}

func (b *Block) GetEpoch() int {
	return util.GetIntFromMap(b.bm, TCEpochKey)
}

// Fetch initiator signature details from the given block
func (b *Block) GetInitiatorSignature() *InitiatorSignature {
	var initiatorSign InitiatorSignature
	s, ok := b.bm[TCInitiatorSignatureKey]
	if !ok || s == nil {
		return nil
	}
	//fetch initiator did
	did := util.GetFromMap(s, InitiatorDID)
	initiatorSign.DID = did.(string)
	//fetch initiator sign type
	signType := util.GetFromMap(s, InitiatorSignType)
	initiatorSign.SignType = int(signType.(uint64))
	//fetch initiator nlss share sign
	nlssShare := util.GetFromMap(s, InitiatorNLSSShare)
	initiatorSign.NLSSShare = nlssShare.(string)
	//fetch initiator private sign
	privSign := util.GetFromMap(s, InitiatorPrivateSign)
	initiatorSign.PrivateSign = privSign.(string)
	//fetch initiator hash / signed data
	signedData := util.GetFromMap(s, InitiatorHash)
	initiatorSign.Hash = signedData.(string)

	return &initiatorSign
}

// Fetch quorums' signature details from the given block
func (b *Block) GetQuorumSignatureList() ([]CreditSignature, error) {
	var quorumSignList []CreditSignature
	s := b.bm[TCQuorumSignatureKey]

	qrmSignListMap, ok := s.([]interface{})
	if !ok {
		fmt.Println("not of type []interface{}")
		return nil, fmt.Errorf("failed to fetch quorums' signature information from block map")
	}
	for _, qrmSignMap := range qrmSignListMap {
		var quorumSig CreditSignature
		// When qrmSignMap is a string (in older versions), qrmSign holds the value as a string
		if qrmSign, ok := qrmSignMap.(string); ok {
			// Unmarshal the JSON string into the struct
			err := json.Unmarshal([]byte(qrmSign), &quorumSig)
			if err != nil {
				fmt.Println(err)
			}
			// in older versions (<v0.0.17), there was no field as sign type and all signatures were NLSS signatures
			if quorumSig.SignType == "" {
				quorumSig.SignType = strconv.Itoa(didmodule.NlssVersion)
			}
		} else {
			//fetch quorum did
			qrmDID := util.GetFromMap(qrmSignMap, CreditSigDID)
			quorumSig.DID = qrmDID.(string)
			// 	//fetch quorum sign type
			signType := util.GetFromMap(qrmSignMap, CreditSigSignType)
			quorumSig.SignType = signType.(string)
			// 	//fetch quorum nlss share sign
			nlssShare := util.GetFromMap(qrmSignMap, CreditSigSignature)
			quorumSig.Signature = nlssShare.(string)
			// 	//fetch quorum private sign
			privSign := util.GetFromMap(qrmSignMap, CreditSigPrivSignature)
			quorumSig.PrivSignature = privSign.(string)
		}
		quorumSignList = append(quorumSignList, quorumSig)
	}

	return quorumSignList, nil
}

// calculate block hash from block data
func (b *Block) CalculateBlockHash() (string, error) {
	var m map[string]interface{}

	err := cbor.Unmarshal(b.bb, &m)
	if err != nil {
		return "", err
	}
	_, iok := m[TCInitiatorSignatureKey]
	if iok {
		delete(m, TCInitiatorSignatureKey)
	}
	_, qok := m[TCQuorumSignatureKey]
	if qok {
		delete(m, TCQuorumSignatureKey)
	}
	bc, ok := m[TCBlockContentKey]
	if !ok {
		return "", fmt.Errorf("invalid block, block content missing")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	blockHash := util.HexToStr(hb)

	return blockHash, nil
}

func (b *Block) GetTokenLevel(token string) (int, int) {
	gtm := b.getGenesisTokenMap(token)
	tokenLevel := util.GetIntFromMap(gtm, GITokenLevelKey)
	tokenNum := util.GetIntFromMap(gtm, GITokenNumberKey)
	return tokenLevel, tokenNum
}

func (b *Block) GetPledgedTokens() {
	pledgedInfo := util.GetFromMap(b.bm, TCPledgeDetailsKey)
	fmt.Println(pledgedInfo)
	// return
}

func (b *Block) SignByInitiator(dc didmodule.DIDCrypto) error {
	did := dc.GetDID()
	blockHash, err := b.GetHash()
	if err != nil {
		return fmt.Errorf("failed to get hash")
	}
	nlssSig, pvtSig, err := dc.Sign(blockHash)
	if err != nil {
		return fmt.Errorf("failed to get initiator signature:%v; err: %v ", did, err.Error())
	}

	initiatorSigMap := make(map[string]interface{})
	initiatorSigMap[InitiatorDID] = did
	initiatorSigMap[InitiatorSignType] = uint64(dc.GetSignType())
	initiatorSigMap[InitiatorNLSSShare] = util.HexToStr(nlssSig)
	initiatorSigMap[InitiatorPrivateSign] = util.HexToStr(pvtSig)
	initiatorSigMap[InitiatorHash] = blockHash
	b.bm[TCInitiatorSignatureKey] = initiatorSigMap

	return b.blkEncode()
}
