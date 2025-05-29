package block

import (
	"fmt"

	"github.com/fxamacker/cbor"
	didmodule "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

type MiningChain struct {
	bb []byte
	bm map[string]interface{}
}

const (
	RubixMiningChainIDString string = "RUBIX_MINING_CHAIN_TEST_PART_4"
	RubixMiningChainID       string = "QmNdtJmgZm584gFQ5wvEpSJAgqE251uMGHkfWPUo5Kmu2D"
)

const (
	MiningChainBlockNumberKey    string = "Block_number"
	MiningChainBlockHashKey      string = "Block_hash"
	MiningChainBlockSignatureKey string = "Block_sign"
	MiningChainBlockInfoKey      string = "Block_info"
)

const (
	MIMiningIDKey              string = "Mining_ID"
	MIMinerDID                 string = "Miner_DID"
	MIMinedTokenIDKey          string = "TokenID"
	MIMinedTokenLevelKey       string = "Token_level"
	MIMinedTokenNumberKey      string = "Token_number"
	MICreditDetailsKey         string = "Credit_details"
	MIPledgeDetailsKey         string = "Quorum_pledge_details"
	MIMiningQuorumSignatureKey string = "Quorum_signature"
	MIEpochKey                 string = "Epoch"
	MIPreviousMiningIDKey      string = "Previous_mining_ID"
)

type MiningChainBlockInfo struct {
	MiningID         string                 `json:"Mining_ID"`
	MinerDID         string                 `json:"Miner_DID"`
	TokenID          string                 `json:"TokenID"`
	TokenLevel       int                    `json:"Token_level"`
	TokenNumber      uint64                 `json:"Token_number"`
	CreditDetails    map[string]interface{} `json:"Credit_details"`
	PledgeDetails    []PledgeDetail         `json:"Quorum_pledge_details"`
	QuorumSignature  []QuorumSignature      `json:"Quorum_signature"`
	Epoch            int                    `json:"Epoch"`
	PreviousMiningID string                 `json:"Previous_mining_ID"`
}

type MiningChainBlock struct {
	MiningChainBlockNumber    uint64                `json:"Block_number"`
	MiningChainBlockHash      string                `json:"Block_hash"`
	MiningChainBlockSignature string                `json:"Block_sign"`
	MiningChainInfo           *MiningChainBlockInfo `json:"Block_info"`
}

func NewMiningInfo(ctcb map[string]*MiningChain, mi *MiningChainBlockInfo) map[string]interface{} {
	nmcbi := make(map[string]interface{})

	nmcbi[MIMiningIDKey] = mi.MiningID
	nmcbi[MIMinerDID] = mi.MinerDID
	nmcbi[MIMinedTokenIDKey] = mi.TokenID
	nmcbi[MIMinedTokenLevelKey] = mi.TokenLevel
	nmcbi[MIMinedTokenNumberKey] = mi.TokenNumber

	if len(mi.CreditDetails) > 0 {
		nmcbi[MICreditDetailsKey] = mi.CreditDetails
	}
	if len(mi.PledgeDetails) > 0 {
		nmcbi[MIPledgeDetailsKey] = mi.PledgeDetails
	}
	if len(mi.QuorumSignature) > 0 {
		nmcbi[MIMiningQuorumSignatureKey] = mi.QuorumSignature
	}
	if mi.Epoch != 0 {
		nmcbi[MIEpochKey] = mi.Epoch
	}
	if mi.PreviousMiningID != "" {
		nmcbi[MIPreviousMiningIDKey] = mi.PreviousMiningID
	}
	return nmcbi
}

func (mb *MiningChain) CreateMiningChainBlock(ctcb map[string]*MiningChain, mcb *MiningChainBlock) *MiningChain {
	nmcb := make(map[string]interface{})
	nmcb[MiningChainBlockNumberKey] = mcb.MiningChainBlockNumber
	nmcb[MiningChainBlockHashKey] = mcb.MiningChainBlockHash
	nmcb[MiningChainBlockSignatureKey] = mcb.MiningChainBlockSignature

	if mcb.MiningChainInfo != nil {
		nmcib := NewMiningInfo(ctcb, mcb.MiningChainInfo)
		nmcb[MiningChainBlockInfoKey] = nmcib
	}
	miningBlock := InitMiningBlock(nil, nmcb)
	return miningBlock
}

// InitMiningBlock initializes a MiningBlock with either serialized bytes or a map.
func InitMiningBlock(bb []byte, bm map[string]interface{}) *MiningChain {
	b := &MiningChain{
		bb: bb,
		bm: bm,
	}
	if b.bb == nil && b.bm == nil {
		return nil
	}
	var err error
	if b.bb == nil {
		err = b.miningBlkEncode()
		if err != nil {
			return nil
		}
	}
	if b.bm == nil {
		err = b.miningBlkDecode()
		if err != nil {
			return nil
		}
	}
	return b
}

// miningBlkEncode serializes the mining block into CBOR bytes, computing the hash in the process.
func (mb *MiningChain) miningBlkEncode() error {
	// Remove hash and signature before CBOR conversion
	_, hok := mb.bm[MiningChainBlockHashKey]
	if hok {
		delete(mb.bm, MiningChainBlockHashKey)
	}
	s, sok := mb.bm[MiningChainBlockSignatureKey]
	if sok {
		delete(mb.bm, MiningChainBlockSignatureKey)
	}
	bc, err := cbor.Marshal(mb.bm, cbor.CanonicalEncOptions())
	if err != nil {
		return fmt.Errorf("failed to serialize block content: %v", err)
	}
	hb := util.CalculateHash(bc, "SHA3-256")
	mb.bm[MiningChainBlockHashKey] = util.HexToStr(hb)
	if sok {
		mb.bm[MiningChainBlockSignatureKey] = s
	}
	m := make(map[string]interface{})
	m["1"] = bc
	if sok {
		ksm, err := cbor.Marshal(s, cbor.CanonicalEncOptions())
		if err != nil {
			return fmt.Errorf("failed to serialize signature: %v", err)
		}
		m["2"] = ksm
	}
	blk, err := cbor.Marshal(m, cbor.CanonicalEncOptions())
	if err != nil {
		return fmt.Errorf("failed to serialize final block: %v", err)
	}
	mb.bb = blk
	return nil
}

// miningBlkDecode deserializes the CBOR bytes into a map representation.
func (mb *MiningChain) miningBlkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(mb.bb, &m)
	if err != nil {
		return fmt.Errorf("failed to decode block: %v", err)
	}
	bc, ok := m["1"]
	if !ok {
		return fmt.Errorf("invalid block, missing block content")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	var mcb map[interface{}]interface{}
	err = cbor.Unmarshal(bc.([]byte), &mcb)
	if err != nil {
		return fmt.Errorf("failed to unmarshal block content: %v", err)
	}
	// Convert tcb to map[string]interface{}
	convertedMcb := make(map[string]interface{})
	for k, v := range mcb {
		key, ok := k.(string)
		if !ok {
			return fmt.Errorf("non-string key found in block content: %v", k)
		}
		convertedMcb[key] = v
	}
	// Handle nested MiningChainBlockInfoKey if it exists
	if info, ok := convertedMcb[MiningChainBlockInfoKey]; ok {
		if infoMap, ok := info.(map[interface{}]interface{}); ok {
			convertedInfo := make(map[string]interface{})
			for k, v := range infoMap {
				key, ok := k.(string)
				if !ok {
					return fmt.Errorf("non-string key found in mining infos: %v", k)
				}
				convertedInfo[key] = v
			}
			convertedMcb[MiningChainBlockInfoKey] = convertedInfo
		}
	}
	if si, sok := m["2"]; sok {
		var ksb string
		err = cbor.Unmarshal(si.([]byte), &ksb)
		if err != nil {
			return fmt.Errorf("failed to unmarshal signature: %v", err)
		}
		convertedMcb[MiningChainBlockSignatureKey] = ksb
	}
	convertedMcb[MiningChainBlockHashKey] = util.HexToStr(hb)
	mb.bm = convertedMcb
	return nil
}

func (mb *MiningChain) GetMiningChainBlockHash() (string, error) {
	// Temporarily remove hash and signature to compute the hash of the content
	hash, hasHash := mb.bm[MiningChainBlockHashKey]
	sig, hasSig := mb.bm[MiningChainBlockSignatureKey]
	if hasHash {
		delete(mb.bm, MiningChainBlockHashKey)
	}
	if hasSig {
		delete(mb.bm, MiningChainBlockSignatureKey)
	}

	// Serialize the remaining map to CBOR
	bc, err := cbor.Marshal(mb.bm, cbor.CanonicalEncOptions())
	if err != nil {
		// Restore the removed fields if serialization fails
		if hasHash {
			mb.bm[MiningChainBlockHashKey] = hash
		}
		if hasSig {
			mb.bm[MiningChainBlockSignatureKey] = sig
		}
		return "", fmt.Errorf("failed to marshal block for hash: %v", err)
	}

	// Compute the hash (SHA3-256, as used in miningBlkEncode)
	hb := util.CalculateHash(bc, "SHA3-256")
	hashStr := util.HexToStr(hb)

	// Restore the removed fields
	if hasHash {
		mb.bm[MiningChainBlockHashKey] = hash
	}
	if hasSig {
		mb.bm[MiningChainBlockSignatureKey] = sig
	}

	return hashStr, nil
}

func (mb *MiningChain) UpdateMiningChainBlockSignature(dc didmodule.DIDCrypto) error {
	// Get the hash of the block
	hashStr, err := mb.GetMiningChainBlockHash()
	if err != nil {
		return fmt.Errorf("failed to get hash: %v", err)
	}

	// Convert the hash string to bytes for signing
	hashBytes := util.StrToHex(hashStr)
	if hashBytes == nil {
		return fmt.Errorf("failed to convert hash to bytes")
	}
	// Sign the hash using the DIDCrypto object
	sb, err := dc.PvtSign(hashBytes)
	if err != nil {
		return fmt.Errorf("failed to sign hash: %v", err)
	}

	// Convert the signature bytes to a hexadecimal string
	sig := util.HexToStr(sb)

	// Store the signature in the map (single signature, not a map of signatures)
	mb.bm[MiningChainBlockSignatureKey] = sig

	// Encode the block with the new signature
	err = mb.miningBlkEncode()
	if err != nil {
		return fmt.Errorf("failed to encode block with new signature: %v", err)
	}

	return nil
}

// GetBlock returns the serialized block bytes.
func (mb *MiningChain) GetMiningBlock() []byte {
	return mb.bb
}

// GetBlockMap retrieves the block map.
func (mb *MiningChain) GetMiningChainBlockMap() (map[string]interface{}, error) {
	return mb.bm, nil
}

// GetBlockNumber retrieves the block number.
func (mb *MiningChain) GetMiningChainBlockNumber() (uint64, error) {
	val, ok := mb.bm[MiningChainBlockNumberKey]
	if !ok {
		return 0, fmt.Errorf("block number key not found")
	}

	switch v := val.(type) {
	case float64:
		if v < 0 || v != float64(uint64(v)) {
			return 0, fmt.Errorf("block number is not a valid uint64: %v", v)
		}
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("block number is negative: %v", v)
		}
		return uint64(v), nil
	case uint64:
		return v, nil
	default:
		return 0, fmt.Errorf("block number is not a number: %T", v)
	}
}

// GetMiningChainID retrieves the mining chain ID.
func GetMiningChainID() string {
	return RubixMiningChainID
}

// GetMiningInfos retrieves the mining info map.
func (mb *MiningChain) GetMiningInfos() (map[string]interface{}, error) {
	val, ok := mb.bm[MiningChainBlockInfoKey]
	if !ok {
		return nil, fmt.Errorf("mining infos key '%s' not found", MiningChainBlockInfoKey)
	}
	infos, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("mining infos is not a map[string]interface{}, got %T: %v", val, val)
	}
	return infos, nil
}

// // GetHash retrieves the block hash.
// func (mb *MiningBlock) GetHash() (string, error) {
// 	hash, ok := mb.bm[MiningChainBlockHashKey].(string)
// 	if !ok {
// 		return "", fmt.Errorf("block hash not found or invalid")
// 	}
// 	return hash, nil
// }

// GetMiningChainBlockSignature retrieves the block signature from MiningChainBlock.
func (mb *MiningChain) GetMiningChainBlockSignature() (string, error) {
	val, ok := mb.bm[MiningChainBlockSignatureKey]
	if !ok {
		return "", fmt.Errorf("block signature key not found")
	}
	sig, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("block signature is not a string")
	}
	return sig, nil
}

// GetMiningID retrieves the MiningID from MiningChainBlockInfo.
func (mb *MiningChain) GetMiningID() (string, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return "", err
	}
	val, ok := infos[MIMiningIDKey]
	if !ok {
		return "", fmt.Errorf("mining ID key not found")
	}
	miningID, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("mining ID is not a string")
	}
	return miningID, nil
}

// GetMinerDID retrieves the MinerDID from MiningChainBlockInfo.
func (mb *MiningChain) GetMinerDID() (string, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return "", err
	}
	val, ok := infos[MIMinerDID]
	if !ok {
		return "", fmt.Errorf("miner DID key not found")
	}
	minerDID, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("miner DID is not a string")
	}
	return minerDID, nil
}

// GetTokenID retrieves the TokenID from MiningChainBlockInfo.
func (mb *MiningChain) GetTokenID() (string, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return "", err
	}
	val, ok := infos[MIMinedTokenIDKey]
	if !ok {
		return "", fmt.Errorf("token ID key not found")
	}
	tokenID, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("token ID is not a string")
	}
	return tokenID, nil
}

// GetTokenLevel retrieves the TokenLevel from MiningChainBlockInfo.
func (mb *MiningChain) GetTokenLevel() (int, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return 0, fmt.Errorf("failed to get mining infos: %v", err)
	}
	val, ok := infos[MIMinedTokenLevelKey]
	if !ok {
		return 0, fmt.Errorf("token level key not found")
	}
	switch v := val.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("token level is not an integer: %v", v)
		}
		return int(v), nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case uint64:
		return int(v), nil // Handle uint64 by converting to int
	default:
		return 0, fmt.Errorf("token level is not a number: %T, value: %v", v, v)
	}
}

// GetTokenNumber retrieves the TokenNumber from MiningChainBlockInfo.
func (mb *MiningChain) GetTokenNumber() (uint64, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return 0, fmt.Errorf("failed to get mining infos: %v", err)
	}
	val, ok := infos[MIMinedTokenNumberKey]
	if !ok {
		return 0, fmt.Errorf("token number key not found")
	}
	switch v := val.(type) {
	case float64:
		if v != float64(uint64(v)) {
			return 0, fmt.Errorf("token number is not an integer: %v", v)
		}
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("token number is negative: %v", v)
		}
		return uint64(v), nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("token number is negative: %v", v)
		}
		return uint64(v), nil
	case uint64:
		return v, nil // Directly return uint64
	default:
		return 0, fmt.Errorf("token number is not a number: %T, value: %v", v, v)
	}
}

// GetCreditDetails retrieves the CreditDetails from MiningChainBlockInfo.
func (mb *MiningChain) GetCreditDetails() (map[string]interface{}, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return nil, fmt.Errorf("failed to get mining infos: %v", err)
	}
	val, ok := infos[MICreditDetailsKey]
	if !ok {
		return nil, nil // No Credit_details
	}

	// Try direct type assertion
	if creditDetails, ok := val.(map[string]interface{}); ok {
		return creditDetails, nil
	}

	// Handle map[interface{}]interface{} from CBOR deserialization
	if genericMap, ok := val.(map[interface{}]interface{}); ok {
		creditDetails := make(map[string]interface{})
		for k, v := range genericMap {
			keyStr, ok := k.(string)
			if !ok {
				fmt.Printf("GetCreditDetails: Non-string key in Credit_details, type %T, value: %v\n", k, k)
				continue
			}
			creditDetails[keyStr] = v
		}
		if len(creditDetails) == 0 {
			fmt.Printf("GetCreditDetails: Converted map is empty, original: %v\n", genericMap)
			return nil, nil
		}
		return creditDetails, nil
	}

	// Log unexpected type for debugging
	fmt.Printf("GetCreditDetails: Credit_details is not a map, got type %T, value: %v\n", val, val)
	return nil, nil
}

// GetPledgeDetails retrieves the PledgeDetails from MiningChainBlockInfo.
func (mb *MiningChain) GetPledgeDetails() ([]interface{}, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return nil, err
	}
	val, ok := infos[MIPledgeDetailsKey]
	if !ok {
		return nil, fmt.Errorf("pledge details key not found")
	}
	pledgeDetails, ok := val.([]interface{})
	if !ok {
		return nil, fmt.Errorf("pledge details is not a slice")
	}
	return pledgeDetails, nil
}

// GetQuorumSignature retrieves the QuorumSignature from MiningChainBlockInfo.
func (mb *MiningChain) GetQuorumSignature() ([]interface{}, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return nil, err
	}
	val, ok := infos[MIMiningQuorumSignatureKey]
	if !ok {
		return nil, fmt.Errorf("quorum signature key not found")
	}
	quorumSignature, ok := val.([]interface{})
	if !ok {
		return nil, fmt.Errorf("quorum signature is not a slice")
	}
	return quorumSignature, nil
}

// GetEpoch retrieves the Epoch from MiningChainBlockInfo.
func (mb *MiningChain) GetEpoch() (int, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return 0, err
	}
	val, ok := infos[MIEpochKey]
	if !ok {
		return 0, fmt.Errorf("epoch key not found")
	}
	switch v := val.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("epoch is not an integer: %v", v)
		}
		return int(v), nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("epoch is not a number: %T", v)
	}
}

// GetPreviousMiningID retrieves the PreviousMiningID from MiningChainBlockInfo.
func (mb *MiningChain) GetPreviousMiningID() (string, error) {
	infos, err := mb.GetMiningInfos()
	if err != nil {
		return "", err
	}
	val, ok := infos[MIPreviousMiningIDKey]
	if !ok {
		return "", fmt.Errorf("previous mining ID key not found")
	}
	prevMiningID, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("previous mining ID is not a string")
	}
	return prevMiningID, nil
}
