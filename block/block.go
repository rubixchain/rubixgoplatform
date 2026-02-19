package block

import (
	"fmt"
	"strconv"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/constants"
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
	TCTokenTypeKey          string = "tokenType"
	TCBlockTypeKey          string = "blockType"
	TCTokenOwnerKey         string = "tokenOwner"
	TCGenesisBlockKey       string = "genesisBlock"
	TCTransInfoKey          string = "transInfo"
	TCQuorumSignatureKey    string = "quorumSignature"
	TCPledgeDetailsKey      string = "pledgeDetails"
	TCBlockHashKey          string = "blockHash"
	TCSignatureKey          string = "signature"
	TCBlockContentKey       string = "blockContent"
	TCBlockContentSigKey    string = "blockContentSig"
	TCDataKey               string = "data"
	TCTokenValueKey         string = "tokenValue"
	TCChildTokensKey        string = "childTokens"
	TCInitiatorSignatureKey string = "initiatorSignature"
	TCEpochKey              string = "epoch"
	TCVersionKey            string = "version"
)

const (
	TokenMintedType          string = "01"
	TokenTransferredType     string = "02"
	TokenMigratedType        string = "03"
	TokenPledgedType         string = "04"
	TokenGeneratedType       string = "05"
	TokenUnpledgedType       string = "06"
	TokenCommittedType       string = "07"
	TokenBurntType           string = "08"
	TokenDeployedType        string = "09"
	TokenExecutedType        string = "10"
	TokenContractCommited    string = "11"
	TokenPinnedAsService     string = "12"
	TokenIsBurntForFT        string = "13"
	TokenSelfTransferredType string = "20" // xell migration
)

const (
	BlockSignatureKey string = "signature"
	BlockSigDIDKey    string = "did"
	BlockSigHashKey   string = "hash"
	BlockSignTypeKey  string = "sign_type"
)

type TokenChainBlock struct {
	BlockType          string           `json:"blockType"`
	TokenOwner         string           `json:"owner"`
	GenesisBlock       *GenesisBlock    `json:"genesisBlock"`
	TransInfo          *TransInfo       `json:"transInfo"`
	PledgeDetails      []PledgeDetail   `json:"pledgeDetails"`
	QuorumSignature    []BlockSignature `json:"quorumSignature"`
	TokenValue         float64          `json:"tokenValue"`
	ChildTokens        []string         `json:"childTokens"`
	InitiatorSignature *BlockSignature  `json:"initiatorSignature"`
	Epoch              int              `json:"epoch"`
	Data               string           `json:"data"`
	Version            int              `json:"version"`
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

// unique signature format for all signatures on block
type BlockSignature struct {
	Signature string `json:"signature"`
	DID       string `json:"did"`
	Hash      string `json:"hash"`
	SignType  int    `json:"sign_type"` //represents sign type (BIPSign == 0)
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
		return nil
	}
	for _, opt := range opts {
		opt(b)
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
	if tcb.TransInfo == nil || ctcb == nil {
		return nil
	}
	ntcb := make(map[string]interface{})
	ntcb[TCBlockTypeKey] = tcb.BlockType
	ntcb[TCTokenOwnerKey] = tcb.TokenOwner
	if tcb.GenesisBlock != nil {
		ntcb[TCGenesisBlockKey] = newGenesisBlock(tcb.GenesisBlock)
		if ntcb[TCGenesisBlockKey] == nil {
			return nil
		}
	}
	ntib := newTransInfo(ctcb, tcb.TransInfo)
	if ntib == nil {
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
	if tcb.Data != "" {
		ntcb[TCDataKey] = tcb.Data
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

	if tcb.Version != 0 {
		ntcb[TCVersionKey] = tcb.Version
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
	blk, err := cbor.Marshal(m, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	b.bb = blk
	return nil
}

func (b *Block) getTokensMap(t string) interface{} {
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
	im := util.GetFromMap(gbm, GenesisInfoKey)
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
	if !ok {
		fmt.Println("*****signature verification failed***")
	}
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

	var sig string

	sb, err := dc.PvtSign([]byte(h))
	if err != nil {
		return fmt.Errorf("failed to get did signature, " + err.Error())
	}
	sig = util.HexToStr(sb)

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
	return b.getBlkString(TCBlockTypeKey)
}

func (b *Block) GetOwner() string {
	return b.getBlkString(TCTokenOwnerKey)
}

func (b *Block) GetSenderDID() string {
	return b.getTrasnInfoString(TISenderDIDKey)
}

//	func (b *Block) GetReceiverDID() string {
//		return b.getBlkString(TCTokenOwnerKey)
//	}
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

func (b *Block) GetParentDetials(t string) (string, error) {
	gtm := b.getGenesisTokenMap(t)
	if gtm == nil {
		return "", fmt.Errorf("invalid token chain block, missing genesis block")
	}
	p := util.GetStringFromMap(gtm, GenesisParentIDKey)
	return p, nil
}

func (b *Block) GetCommitedTokenDetials(t string) ([]string, error) {
	genesisTokenMap := b.getGenesisTokenMap(t)
	if genesisTokenMap == nil {
		return nil, fmt.Errorf("invalid token chain block, missing genesis block")
	}
	commitedTokensMap := util.GetFromMap(genesisTokenMap, GenesisCommitedTokensKey)
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

func (b *Block) GetDataFromTokenChain() string {
	return b.getBlkString(TCDataKey)
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

func (b *Block) GetVersion() int {
	return util.GetIntFromMap(b.bm, TCVersionKey)
}

// Fetch initiator signature details from the given block
func (b *Block) GetInitiatorSignature() *BlockSignature {
	var initiatorSign BlockSignature
	s, ok := b.bm[TCInitiatorSignatureKey]
	if !ok || s == nil {
		return nil
	}
	//fetch initiator did
	did := util.GetFromMap(s, BlockSigDIDKey)
	initiatorSign.DID = did.(string)
	//fetch initiator sign type
	signType := util.GetFromMap(s, BlockSignTypeKey)
	initiatorSign.SignType = int(signType.(uint64))
	//fetch initiator sign
	privSign := util.GetFromMap(s, BlockSignatureKey)
	initiatorSign.Signature = privSign.(string)
	//fetch initiator hash / signed data
	signedData := util.GetFromMap(s, BlockSigHashKey)
	initiatorSign.Hash = signedData.(string)

	return &initiatorSign
}

// Fetch quorums' signature details from the given block
func (b *Block) GetQuorumSignatureList() ([]BlockSignature, error) {
	var quorumSignList []BlockSignature
	s := b.bm[TCQuorumSignatureKey]

	qrmSignListMap, ok := s.([]interface{})
	if !ok {
		fmt.Println("not of type []interface{}")
		return nil, fmt.Errorf("failed to fetch quorums' signature information from block map")
	}
	for _, qrmSignMap := range qrmSignListMap {
		var quorumSig BlockSignature
		//fetch quorum did
		qrmDID := util.GetFromMap(qrmSignMap, BlockSigDIDKey)
		quorumSig.DID = qrmDID.(string)
		//fetch quorum sign type
		signType := util.GetFromMap(qrmSignMap, BlockSignTypeKey)
		quorumSig.SignType = int(signType.(uint64))
		//fetch quorum sign
		privSign := util.GetFromMap(qrmSignMap, BlockSignatureKey)
		quorumSig.Signature = privSign.(string)

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
	bc, ok := m[TCBlockContentKey]
	if !ok {
		return "", fmt.Errorf("invalid block, block content missing")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	blockHash := util.HexToStr(hb)

	return blockHash, nil
}

func (b *Block) GetPledgedTokens() []PledgeDetail {
	pledgedInfo := util.GetFromMap(b.bm, TCPledgeDetailsKey)
	if pledgedInfo == nil {
		return nil
	}

	pledgedMap, ok := pledgedInfo.(map[string]interface{})
	if !ok {
		return nil
	}

	ptds := make([]PledgeDetail, 0)
	for did, tokens := range pledgedMap {
		switch tokenSlice := tokens.(type) {
		case []map[string]interface{}:
			for _, t := range tokenSlice {
				ptds = append(ptds, PledgeDetail{
					DID:   did,
					Token: fmt.Sprint(t[PDTokenKey]),
					TokenType: func(v interface{}) int {
						switch val := v.(type) {
						case int:
							return val
						case int32:
							return int(val)
						case int64:
							return int(val)
						case float64:
							return int(val)
						case string:
							i, _ := strconv.Atoi(val)
							return i
						default:
							return 0
						}
					}(t[PDTokenTypeKey]),
					TokenBlockID: fmt.Sprint(t[PDTokenBlockIDKey]),
				})
			}
		case []interface{}: // defensive: JSON unmarshal case
			for _, raw := range tokenSlice {
				if t, ok := raw.(map[string]interface{}); ok {
					ptds = append(ptds, PledgeDetail{
						DID:   did,
						Token: fmt.Sprint(t[PDTokenKey]),
						TokenType: func(v interface{}) int {
							switch val := v.(type) {
							case int:
								return val
							case int32:
								return int(val)
							case int64:
								return int(val)
							case float64:
								return int(val)
							case string:
								i, _ := strconv.Atoi(val)
								return i
							default:
								return 0
							}
						}(t[PDTokenTypeKey]),
						TokenBlockID: fmt.Sprint(t[PDTokenBlockIDKey]),
					})
				}
			}
		}
	}

	return ptds
}

func (b *Block) GetGenesisNetworkType(t string) (string, error) {
	genesisInfoMap := b.getGenesisTokenMap(t)

	networkID := util.GetFromMap(genesisInfoMap, GenesisNetworkIDKey)
	if networkID == nil {
		return "", fmt.Errorf("network ID not found in genesis info")
	}

	networkIDStr, ok := networkID.(string)
	if !ok {
		return "", fmt.Errorf("network ID is not a string")
	}

	switch networkIDStr {
	case constants.NetworkID_RBT_Local, constants.NetworkID_RBT_Testnet, constants.NetworkID_RBT_Mainnet:
		// valid network IDs
	default:
		return "", fmt.Errorf("invalid network ID: %s", networkIDStr)
	}

	return networkIDStr, nil
}
