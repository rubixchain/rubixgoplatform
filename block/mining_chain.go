package block

import (
	"fmt"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/util"
)

type MiningBlock struct {
	bb []byte
	bm map[string]interface{}
}

const (
	MiningChainBlockNumberKey    string = "1"
	MiningChainBlockHashKey      string = "2"
	MiningChainBlockSignatureKey string = "3"
	MiningChainBlockInfoKey      string = "4"
)

const (
	MIMiningIDKey              string = "1"
	MIMinerDID                 string = "2"
	MIMinedTokenIDKey          string = "3"
	MIMinedTokenLevelKey       string = "4"
	MIMinedTokenNumberKey      string = "5"
	MICreditDeatilsKey         string = "6"
	MIPledgeDetailsKey         string = "7"
	MIMiningQuorumSignatureKey string = "8"
	MIEpochKey                 string = "9"
	MIPreviousMiningIDKey      string = "10"
)

type MiningChainBlockInfo struct {
	MiningID         string                 `json:"miningID"`
	MinerDID         string                 `json:"minerDID"`
	TokenID          string                 `json:"tokenID"`
	TokenLevel       int                    `json:"tokenLevel"`
	TokenNumber      int                    `json:"tokenNumber"`
	CreditDetails    map[string]interface{} `json:"creditDetails"`
	PledgeDetails    []PledgeDetail         `json:"pledgeDetails"`
	QuorumSignature  []CreditSignature      `json:"quorumSignature"`
	Epoch            int                    `json:"epoch"`
	PreviousMiningID string                 `json:"previousMiningID"`
}

type MiningChainBlock struct {
	MiningChainBlockNumber    int                   `json:"miningBlockNumber"`
	MiningChainBlockHash      string                `json:"miningChainBlockHash"`
	MiningChainBlockSignature string                `json:"miningChainBlockSignature"`
	MiningChainInfo           *MiningChainBlockInfo `json:"miningChainInfo"`
}

func NewMiningInfo(ctcb map[string]*MiningBlock, mi *MiningChainBlockInfo) map[string]interface{} {
	nmcbi := make(map[string]interface{})

	nmcbi[MIMiningIDKey] = mi.MiningID
	nmcbi[MIMinerDID] = mi.MinerDID
	nmcbi[MIMinedTokenIDKey] = mi.TokenID
	nmcbi[MIMinedTokenLevelKey] = mi.TokenLevel
	nmcbi[MIMinedTokenNumberKey] = mi.TokenNumber

	if len(mi.CreditDetails) > 0 {
		nmcbi[MICreditDeatilsKey] = mi.CreditDetails
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

func (mb *MiningBlock) CreateMiningChainBlock(ctcb map[string]*MiningBlock, mcb *MiningChainBlock) *MiningBlock {
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
func InitMiningBlock(bb []byte, bm map[string]interface{}) *MiningBlock {
	b := &MiningBlock{
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
func (mb *MiningBlock) miningBlkEncode() error {
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
func (mb *MiningBlock) miningBlkDecode() error {
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

// // GetBlock returns the serialized block bytes.
// func (mb *MiningBlock) GetMiningBlock() []byte {
// 	return mb.bb
// }

// // GetBlockNumber retrieves the block number.
// func (mb *MiningBlock) GetMiningChainBlockNumber() (int, error) {
// 	bn, ok := mb.bm[MiningChainBlockNumberKey].(float64)
// 	if !ok {
// 		return 0, fmt.Errorf("block number not found or invalid")
// 	}
// 	return int(bn), nil
// }

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
