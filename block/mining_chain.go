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
	RubixMiningChainIDString string = "rubix_mining_chain_test_"
	RubixMiningChainID       string = "QmT4HbZiQU6QXvVFvrKH6HSLP4PdxbVBDsseCr6jXq9AMM"
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
	MICreditDeatilsKey         string = "Credit_details"
	MIPledgeDetailsKey         string = "Quorum_pledge_details"
	MIMiningQuorumSignatureKey string = "Quorum_signature"
	MIEpochKey                 string = "Epoch"
	MIPreviousMiningIDKey      string = "Previous_mining_ID"
)

type MiningChainBlockInfo struct {
	MiningID         string                 `json:"miningID"`
	MinerDID         string                 `json:"minerDID"`
	TokenID          string                 `json:"tokenID"`
	TokenLevel       int                    `json:"tokenLevel"`
	TokenNumber      int                    `json:"tokenNumber"`
	CreditDetails    map[string]interface{} `json:"creditDetails"`
	PledgeDetails    []PledgeDetail         `json:"pledgeDetails"`
	QuorumSignature  []QuorumSignature      `json:"quorumSignature"`
	Epoch            int                    `json:"epoch"`
	PreviousMiningID string                 `json:"previousMiningID"`
}

type MiningChainBlock struct {
	MiningChainBlockNumber    uint64                `json:"miningBlockNumber"`
	MiningChainBlockHash      string                `json:"miningChainBlockHash"`
	MiningChainBlockSignature string                `json:"miningChainBlockSignature"`
	MiningChainInfo           *MiningChainBlockInfo `json:"miningChainInfo"`
}

func NewMiningInfo(ctcb map[string]*MiningChain, mi *MiningChainBlockInfo) map[string]interface{} {
	nmcbi := make(map[string]interface{})

	nmcbi[MIMiningIDKey] = mi.MiningID
	nmcbi[MIMinerDID] = mi.MinerDID
	nmcbi[MIMinedTokenIDKey] = mi.TokenID
	nmcbi[MIMinedTokenLevelKey] = mi.TokenLevel
	nmcbi[MIMinedTokenNumberKey] = mi.TokenNumber

	if len(mi.CreditDetails) > 0 {
		nmcbi[MICreditDeatilsKey] = mi.CreditDetails
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
	var tcb map[string]interface{}
	err = cbor.Unmarshal(bc.([]byte), &tcb)
	if err != nil {
		return fmt.Errorf("failed to unmarshal block content: %v", err)
	}
	if si, sok := m["2"]; sok {
		var ksb string
		err = cbor.Unmarshal(si.([]byte), &ksb)
		if err != nil {
			return fmt.Errorf("failed to unmarshal signature: %v", err)
		}
		tcb[MiningChainBlockSignatureKey] = ksb
	}
	tcb[MiningChainBlockHashKey] = util.HexToStr(hb)
	mb.bm = tcb
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

func GetMiningChainID() string {
	return RubixMiningChainID
}

// // GetHash retrieves the block hash.
// func (mb *MiningBlock) GetHash() (string, error) {
// 	hash, ok := mb.bm[MiningChainBlockHashKey].(string)
// 	if !ok {
// 		return "", fmt.Errorf("block hash not found or invalid")
// 	}
// 	return hash, nil
// }

// // GetSignature retrieves the block signature.
// func (mb *MiningBlock) GetSignature() (string, error) {
// 	sig, ok := mb.bm[MiningChainBlockSignatureKey].(string)
// 	if !ok {
// 		return "", fmt.Errorf("block signature not found or invalid")
// 	}
// 	return sig, nil
// }

// // GetMiningInfos retrieves the mining info map.
// func (mb *MiningBlock) GetMiningInfos() (map[string]interface{}, error) {
// 	infos, ok := mb.bm[MiningChainBlockInfoKey].(map[string]interface{})
// 	if !ok {
// 		return nil, fmt.Errorf("mining infos not found or invalid")
// 	}
// 	return infos, nil
// }
