package rac

import (
	"fmt"

	"github.com/fxamacker/cbor"
	didmodule "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	RacTestTokenType int = iota
	RacOldNFTType
	RacNFTType
	RacDataTokenType
	RacPartTokenType
	RacTestNFTType
	RacTestDataTokenType
	RacTestPartTokenType
)

const (
	RacVersion int = 1
)

const (
	RacTypeKey         string = "1"
	RacVersionKey      string = "2"
	RacDidKey          string = "3"
	RacTokenNumberKey  string = "4"
	RacTotalSupplyKey  string = "5"
	RacTimeStampKey    string = "6"
	RacCreatorIDKey    string = "7"
	RacCreatorInputKey string = "8"
	RacContentIDKey    string = "9"
	RacContentHashKey  string = "10"
	RacContentURLKey   string = "11"
	RacTransInfoKey    string = "12"
	RacPartInfoKey     string = "13"
	RacHashKey         string = "98"
	RacSignKey         string = "99"
	RacBlockCotent     string = "1"
	RacBlockSig        string = "2"
)

const (
	RacPIParentKey  string = "1"
	RacPIPartNumKey string = "2"
	RacPIValueKey   string = "3"
)

type RacType struct {
	Type         int
	DID          string
	TokenNumber  uint64
	TotalSupply  uint64
	TimeStamp    string
	CreatorID    string
	CreatorInput string
	ContentID    map[string]string
	ContentHash  map[string]string
	ContentURL   map[string]string
	TransInfo    map[string]string
	PartInfo     *RacPartInfo
}

type RacPartInfo struct {
	Parent  string
	PartNum int
	Value   float64
}

type RacBlock struct {
	bb []byte
	bm map[string]interface{}
}

func InitRacBlock(bb []byte, bm map[string]interface{}) (*RacBlock, error) {
	r := &RacBlock{
		bb: bb,
		bm: bm,
	}
	if (r.bb == nil && r.bm == nil) || (r.bb != nil && r.bm != nil) {
		return nil, fmt.Errorf("invalid initialization, required valid input")
	}
	if r.bb == nil {
		err := r.blkEncode()
		if err != nil {
			return nil, err
		}
	}
	if r.bm == nil {
		err := r.blkDecode()
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func CreateRac(r *RacType) ([]*RacBlock, error) {
	if r.Type == 1 || r.Type > RacTestPartTokenType {
		return nil, fmt.Errorf("rac type is not supported")
	}
	rb := make([]*RacBlock, 0)
	for i := uint64(0); i < r.TotalSupply; i++ {
		m := make(map[string]interface{})
		m[RacTypeKey] = r.Type
		m[RacVersionKey] = RacVersion
		m[RacDidKey] = r.DID
		m[RacTokenNumberKey] = i
		m[RacTotalSupplyKey] = r.TotalSupply
		if r.CreatorInput != "" {
			m[RacCreatorInputKey] = r.CreatorInput
		}
		if r.CreatorID != "" {
			m[RacCreatorIDKey] = r.CreatorID
		}
		if r.TimeStamp != "" {
			m[RacTimeStampKey] = r.TimeStamp
		}
		if r.ContentID != nil {
			m[RacContentIDKey] = r.ContentID
		}
		if r.ContentHash != nil {
			m[RacContentHashKey] = r.ContentHash
		}
		if r.ContentURL != nil {
			m[RacContentURLKey] = r.ContentURL
		}
		if r.TransInfo != nil {
			m[RacTransInfoKey] = r.TransInfo
		}
		if r.PartInfo != nil {
			m[RacPartInfoKey] = newPartInfo(r.PartInfo)
			if m[RacPartInfoKey] == nil {
				return nil, fmt.Errorf("failed to create part info")
			}
		}
		r, err := InitRacBlock(nil, m)
		if err != nil {
			return nil, err
		}
		rb = append(rb, r)
	}
	return rb, nil
}

func newPartInfo(ti *RacPartInfo) map[string]interface{} {
	tim := make(map[string]interface{})
	if ti.Parent == "" {
		return nil
	}
	tim[RacPIParentKey] = ti.Parent
	tim[RacPIPartNumKey] = ti.PartNum
	tim[RacPIValueKey] = ti.Value
	return tim
}

func (r *RacBlock) blkEncode() error {
	_, ok := r.bm[RacHashKey]
	if ok {
		delete(r.bm, RacHashKey)
	}
	s, sok := r.bm[RacSignKey]
	if sok {
		delete(r.bm, RacSignKey)
	}
	b, err := cbor.Marshal(r.bm, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	h := util.CalculateHash(b, "SHA3-256")
	r.bm[RacHashKey] = util.HexToStr(h)
	nm := make(map[string]interface{})
	nm[RacBlockCotent] = b
	if sok {
		nm[RacBlockSig] = s
	}
	blk, err := cbor.Marshal(nm, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	r.bb = blk
	return nil
}

func (r *RacBlock) blkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(r.bb, &m)
	if err != nil {
		return nil
	}
	_, ok := m[RacBlockSig]
	if !ok {
		return fmt.Errorf("invalid block, missing signature")
	}
	bc, ok := m[RacBlockCotent]
	if !ok {
		return fmt.Errorf("invalid block, missing block content")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	var tcb map[string]interface{}
	err = cbor.Unmarshal(bc.([]byte), &tcb)
	if err != nil {
		return err
	}
	tcb[RacHashKey] = util.HexToStr(hb)
	tcb[RacSignKey] = util.GetStringFromMap(m, RacBlockSig)
	r.bm = tcb
	return nil
}

func (r *RacBlock) GetBlock() []byte {
	return r.bb
}

func (r *RacBlock) GetRacMap() map[string]interface{} {
	return r.bm
}

func (r *RacBlock) UpdateSignature(dc didmodule.DIDCrypto) error {
	ha, err := r.GetHash()
	if err != nil {
		return err
	}
	sig, err := dc.PvtSign([]byte(ha))
	if err != nil {
		return err
	}
	r.bm[RacSignKey] = util.HexToStr(sig)
	return r.blkEncode()
}

func (r *RacBlock) VerifySignature(dc didmodule.DIDCrypto) error {
	ha, sig, err := r.GetHashSig()
	if err != nil {
		return err
	}
	ok, err := dc.PvtVerify([]byte(ha), util.StrToHex(sig))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("failed to verify the rac signature")
	}
	return nil
}

func (r *RacBlock) GetRacType() int {
	return util.GetIntFromMap(r.bm, RacTypeKey)
}

func (r *RacBlock) GetHash() (string, error) {
	h, ok := r.bm[RacHashKey]
	if !ok {
		return "", fmt.Errorf("invalid rac, hash is missing")
	}
	return h.(string), nil
}

func (r *RacBlock) GetDID() string {
	h, ok := r.bm[RacDidKey]
	if !ok {
		return ""
	}
	return h.(string)
}

func (r *RacBlock) GetHashSig() (string, string, error) {
	h, ok := r.bm[RacHashKey]
	if !ok {
		return "", "", fmt.Errorf("invalid rac, hash is missing")
	}
	s, ok := r.bm[RacSignKey]
	if !ok {
		return "", "", fmt.Errorf("invalid rac, signature is missing")
	}
	return h.(string), s.(string), nil
}

func (r *RacBlock) GetRacValue() float64 {
	pi, ok := r.bm[RacPartInfoKey]
	if !ok {
		return 0
	}
	return util.GetFloatFromMap(pi, RacPIValueKey)
}

func RacType2TokenType(rt int) int {
	switch rt {
	case RacTestTokenType:
		return token.TestTokenType
	case RacNFTType:
		return token.NFTTokenType
	case RacTestNFTType:
		return token.TestNFTTokenType
	case RacPartTokenType:
		return token.PartTokenType
	case RacTestPartTokenType:
		return token.TestPartTokenType
	case RacDataTokenType:
		return token.DataTokenType
	case RacTestDataTokenType:
		return token.TestDataTokenType
	}
	return token.RBTTokenType
}
