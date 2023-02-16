package contract

import (
	"fmt"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	SCRBTDirectType int = iota
)
const (
	SCTypeKey           string = "1"
	SCWholeTokensKey    string = "2"
	SCWholeTokensIDKey  string = "3"
	SCPartTokensKey     string = "4"
	SCPartTokensIDKey   string = "5"
	SCSenderDIDKey      string = "6"
	SCReceiverDIDKey    string = "7"
	SCCommentKey        string = "8"
	SCPledgeModeKey     string = "9"
	SCPledgeDetialsKey  string = "10"
	SCShareSignatureKey string = "97"
	SCKeySignatureKey   string = "98"
	SCBlockHashKey      string = "99"

	SCBlockContentKey     string = "1"
	SCBlockContentSSigKey string = "2"
	SCBlockContentPSigKey string = "3"
)

type ContractType struct {
	Type          int                    `json:"type"`
	WholeTokens   []string               `json:"whole_tokens"`
	WholeTokensID []string               `json:"whole_tokens_id"`
	PartTokens    []string               `json:"part_tokens"`
	PartTokensID  []string               `json:"part_tokens_id"`
	SenderDID     string                 `json:"sender_did"`
	ReceiverDID   string                 `json:"receiver_did"`
	PledgeMode    int                    `json:"pledge_mode"`
	PledgeDetials map[string]interface{} `json:"pledge_detials"`
	Comment       string                 `json:"comment"`
}

type Contract struct {
	st uint64
	sb []byte
	sm map[string]interface{}
}

func CreateNewContract(st *ContractType) *Contract {

	nm := make(map[string]interface{})
	nm[SCTypeKey] = st.Type
	if len(st.WholeTokens) > 0 {
		nm[SCWholeTokensKey] = st.WholeTokens
	}
	if len(st.WholeTokensID) > 0 {
		nm[SCWholeTokensIDKey] = st.WholeTokensID
	}
	if len(st.PartTokens) > 0 {
		nm[SCPartTokensKey] = st.PartTokens
	}
	if len(st.PartTokensID) > 0 {
		nm[SCPartTokensIDKey] = st.PartTokensID
	}
	if st.SenderDID != "" {
		nm[SCSenderDIDKey] = st.SenderDID
	}
	if st.ReceiverDID != "" {
		nm[SCReceiverDIDKey] = st.ReceiverDID
	}
	nm[SCCommentKey] = st.Comment
	// ::TODO:: Need to support other pledge mode
	if st.PledgeMode > POWPledgeMode {
		return nil
	}
	nm[SCPledgeModeKey] = st.PledgeMode
	if st.PledgeDetials != nil {
		nm[SCPledgeDetialsKey] = st.PledgeDetials
	}
	return InitContract(nil, nm)
}

func (c *Contract) blkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(c.sb, &m)
	if err != nil {
		return nil
	}
	ssi, ok := m[SCBlockContentSSigKey]
	if !ok {
		return fmt.Errorf("invalid block, missing share signature")
	}
	ksi, ok := m[SCBlockContentPSigKey]
	if !ok {
		return fmt.Errorf("invalid block, missing key signature")
	}
	bc, ok := m[SCBlockContentKey]
	if !ok {
		return fmt.Errorf("invalid block, missing block content")
	}
	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	var tcb map[string]interface{}
	err = cbor.Unmarshal(bc.([]byte), &tcb)
	if err != nil {
		return err
	}
	tcb[SCBlockHashKey] = util.HexToStr(hb)
	tcb[SCShareSignatureKey] = util.HexToStr(ssi.([]byte))
	tcb[SCKeySignatureKey] = util.HexToStr(ksi.([]byte))
	c.sm = tcb
	return nil
}

func (c *Contract) blkEncode() error {
	// Remove Hash & Signature before CBOR conversation
	_, hok := c.sm[SCBlockHashKey]
	if hok {
		delete(c.sm, SCBlockHashKey)
	}
	ss, ssok := c.sm[SCShareSignatureKey]
	if ssok {
		delete(c.sm, SCShareSignatureKey)
	}
	ks, ksok := c.sm[SCKeySignatureKey]
	if ksok {
		delete(c.sm, SCKeySignatureKey)
	}
	bc, err := cbor.Marshal(c.sm, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	hb := util.CalculateHash(bc, "SHA3-256")
	c.sm[SCBlockHashKey] = util.HexToStr(hb)
	m := make(map[string]interface{})
	m[SCBlockContentKey] = bc
	if ssok {
		c.sm[SCShareSignatureKey] = ss
		m[SCBlockContentSSigKey] = util.StrToHex(ss.(string))
	}
	if ksok {
		c.sm[SCKeySignatureKey] = ss
		m[SCBlockContentPSigKey] = util.StrToHex(ks.(string))
	}
	blk, err := cbor.Marshal(m, cbor.CanonicalEncOptions())
	if err != nil {
		return err
	}
	c.sb = blk
	return nil
}

func InitContract(sb []byte, sm map[string]interface{}) *Contract {
	c := &Contract{
		sb: sb,
		sm: sm,
	}
	if c.sb == nil && c.sm == nil {
		return nil
	}
	var err error
	if c.sb == nil {
		err = c.blkEncode()
		if err != nil {
			return nil
		}
	}
	if c.sm == nil {
		err = c.blkDecode()
		if err != nil {
			return nil
		}
	}
	t, ok := c.sm[SCTypeKey]
	if ok {
		c.st, ok = t.(uint64)
		if !ok {
			ti, ok := t.(int)
			if !ok {
				return nil
			}
			c.st = uint64(ti)
		}
	}
	return c
}

func (c *Contract) GetType() uint64 {
	return c.st
}

func (c *Contract) GetHashSig() (string, string, string, error) {
	hi, ok := c.sm[SCBlockHashKey]
	if !ok {
		return "", "", "", fmt.Errorf("invalid smart contract, hash block is missing")
	}

	ssi, ok := c.sm[SCShareSignatureKey]
	if !ok {
		return "", "", "", fmt.Errorf("invalid smart contract, share signature block is missing")
	}
	ksi, ok := c.sm[SCKeySignatureKey]
	if !ok {
		return "", "", "", fmt.Errorf("invalid smart contract, key signature block is missing")
	}
	return hi.(string), ssi.(string), ksi.(string), nil
}

func (c *Contract) GetHash() (string, error) {
	hi, ok := c.sm[SCBlockHashKey]
	if !ok {
		return "", fmt.Errorf("invalid smart contract, hash block is missing")
	}
	return hi.(string), nil
}

func (c *Contract) GetBlock() []byte {
	return c.sb
}

func (c *Contract) GetMap() map[string]interface{} {
	return c.sm
}

func (c *Contract) UpdateSignature(ss []byte, ks []byte) error {
	c.sm[SCShareSignatureKey] = util.HexToStr(ss)
	c.sm[SCKeySignatureKey] = util.HexToStr(ks)
	return c.blkEncode()
}

func (c *Contract) GetPledgeMode() int {
	mi, ok := c.sm[SCPledgeModeKey]
	// Default mode is POW
	if !ok {
		return POWPledgeMode
	}
	return mi.(int)
}

func (c *Contract) GetSenderDID() string {
	si, ok := c.sm[SCSenderDIDKey]
	if !ok {
		return ""
	}
	return si.(string)
}

func (c *Contract) GetReceiverDID() string {
	si, ok := c.sm[SCReceiverDIDKey]
	if !ok {
		return ""
	}
	return si.(string)
}

func (c *Contract) GetWholeTokens() []string {
	wti, ok := c.sm[SCWholeTokensKey]
	if !ok {
		return nil
	}
	wt, ok := wti.([]string)
	if ok {
		return wt
	}
	wtf := wti.([]interface{})
	wt = make([]string, 0)
	for _, i := range wtf {
		wt = append(wt, i.(string))
	}
	return wt
}

func (c *Contract) GetWholeTokensID() []string {
	wti, ok := c.sm[SCWholeTokensIDKey]
	if !ok {
		return nil
	}
	wt, ok := wti.([]string)
	if ok {
		return wt
	}
	wtf := wti.([]interface{})
	wt = make([]string, 0)
	for _, i := range wtf {
		wt = append(wt, i.(string))
	}
	return wt
}

func (c *Contract) GetPartTokens() []string {
	pti, ok := c.sm[SCPartTokensKey]
	if !ok {
		return nil
	}
	wt, ok := pti.([]string)
	if ok {
		return wt
	}
	wtf := pti.([]interface{})
	wt = make([]string, 0)
	for _, i := range wtf {
		wt = append(wt, i.(string))
	}
	return wt
}

func (c *Contract) GetPartTokensID() []string {
	pti, ok := c.sm[SCPartTokensIDKey]
	if !ok {
		return nil
	}
	wt, ok := pti.([]string)
	if ok {
		return wt
	}
	wtf := pti.([]interface{})
	wt = make([]string, 0)
	for _, i := range wtf {
		wt = append(wt, i.(string))
	}
	return wt
}

func (c *Contract) GetComment() string {
	si, ok := c.sm[SCCommentKey]
	if !ok {
		return ""
	}
	return si.(string)
}
