package rac

import (
	"fmt"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	RacTestTokenType int = iota
	RacOldNFTType
	RacNFTType
)

const (
	RacVersion int = 1
)

const (
	RacTestTokenVersion int = 1
)

const (
	RacTypeKey         string = "type"
	RacDidKey          string = "creatorDid"
	RacTotalSupplyKey  string = "totalSupply"
	RacTokenCountKey   string = "tokenCount"
	RacCreatorInputKey string = "creatorInput"
	RacContentHashKey  string = "contentHash"
	RacUrlKey          string = "url"
	RacVersionKey      string = "version"
	RacSignKey         string = "pvtKeySign"
	RacHashKey         string = "hash"
	RacBlockCotent     string = "contentBlock"
	RacBlockSig        string = "contentSig"
)

type RacType struct {
	Type         int
	DID          string
	TotalSupply  uint64
	CreatorInput string
	ContentHash  string
	Url          string
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
	if r.Type == 1 || r.Type > 2 {
		return nil, fmt.Errorf("rac type is not supported")
	}
	rb := make([]*RacBlock, 0)
	for i := uint64(0); i < r.TotalSupply; i++ {
		m := make(map[string]interface{})
		m[RacTypeKey] = r.Type
		m[RacDidKey] = r.DID
		m[RacTotalSupplyKey] = r.TotalSupply
		m[RacTokenCountKey] = i
		m[RacContentHashKey] = r.ContentHash
		m[RacCreatorInputKey] = r.CreatorInput
		m[RacUrlKey] = r.Url
		m[RacVersionKey] = RacVersion
		r, err := InitRacBlock(nil, m)
		if err != nil {
			return nil, err
		}
		rb = append(rb, r)
	}
	return rb, nil
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
	si, ok := m[RacBlockSig]
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
	tcb[RacSignKey] = util.HexToStr(si.([]byte))
	r.bm = tcb
	return nil
}

func (r *RacBlock) GetBlock() []byte {
	return r.bb
}

func (r *RacBlock) GetRacMap() map[string]interface{} {
	return r.bm
}

func (r *RacBlock) UpdateSignature(sig string) error {
	r.bm[RacSignKey] = sig
	return r.blkEncode()
}

func (r *RacBlock) GetHash() (string, error) {
	h, ok := r.bm[RacHashKey]
	if !ok {
		return "", fmt.Errorf("invalid rac, hash is missing")
	}
	return h.(string), nil
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
