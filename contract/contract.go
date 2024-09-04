package contract

import (
	"fmt"

	"github.com/fxamacker/cbor"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	SCRBTDirectType int = iota
	SCDIDMigrateType
	SCDataTokenType
	SCDataTokenCommitType
	SCNFTSaleContractType
	SmartContractDeployType
)

// ----------SmartContract----------------------
// {
// 	 "1"  : Type             : int
// 	 "2"  : PledgeMode       : int
// 	 "3"  : TransInfo        : TransInfo
// 	 "4"  : TotalRBTs        : flaot64
// }

const (
	SCTypeKey             string = "1"
	SCPledgeModeKey       string = "2"
	SCTransInfoKey        string = "3"
	SCTotalRBTsKey        string = "4"
	SCShareSignatureKey   string = "97"
	SCKeySignatureKey     string = "98"
	SCBlockHashKey        string = "99"
	SCBlockContentKey     string = "1"
	SCBlockContentSSigKey string = "2"
	SCBlockContentPSigKey string = "3"
)

type ContractType struct {
	Type       int        `json:"type"`
	PledgeMode int        `json:"pledge_mode"`
	TransInfo  *TransInfo `json:"transInfo"`
	TotalRBTs  float64    `json:"totalRBTs"`
	ReqID      string     `json:"req_id"`
	log        logger.Logger
}

type Contract struct {
	st  uint64
	sb  []byte
	sm  map[string]interface{}
	log logger.Logger
}

func CreateNewContract(st *ContractType) *Contract {
	if st.TransInfo == nil {
		return nil
	}
	//	st.log.Debug("Creating new contract")
	//	st.log.Debug("input st is %v", st)
	//	st.log.Debug("st.TransInfo is %v", st.TransInfo)

	nm := make(map[string]interface{})
	nm[SCTypeKey] = st.Type
	// ::TODO:: Need to support other pledge mode
	if st.PledgeMode > NoPledgeMode {
		return nil
	}
	nm[SCPledgeModeKey] = st.PledgeMode
	nm[SCTransInfoKey] = newTransInfoBlock(st.TransInfo)
	if nm[SCTransInfoKey] == nil {
		return nil
	}
	nm[SCTotalRBTsKey] = st.TotalRBTs
	return InitContract(nil, nm)
}

func (c *Contract) blkDecode() error {
	var m map[string]interface{}
	err := cbor.Unmarshal(c.sb, &m)
	if err != nil {
		return nil
	}
	ssi, sok := m[SCBlockContentSSigKey]
	ksi, kok := m[SCBlockContentPSigKey]
	bc, ok := m[SCBlockContentKey]
	if !ok {
		return fmt.Errorf("invalid block, missing block content")
	}
	/* c.log.Debug("bc is %v", bc)
	c.log.Debug("SCBlockContentPSigKey is %v", ksi)
	c.log.Debug("SCBlockContentPSigKey is %v", ssi) */

	hb := util.CalculateHash(bc.([]byte), "SHA3-256")
	var tcb map[string]interface{}
	err = cbor.Unmarshal(bc.([]byte), &tcb)
	if err != nil {
		return err
	}

	tcb[SCBlockHashKey] = util.HexToStr(hb)
	if sok {
		var ksb map[string]interface{}
		err = cbor.Unmarshal(ssi.([]byte), &ksb)
		if err != nil {
			return err
		}
		//c.log.Debug("ksb is %v", ksb)
		tcb[SCShareSignatureKey] = ksb
	}
	if kok {
		var ksb map[string]interface{}
		err = cbor.Unmarshal(ksi.([]byte), &ksb)
		if err != nil {
			return err
		}
		//c.log.Debug("ksb is %v", ksb)
		tcb[SCKeySignatureKey] = ksb
	}
	c.sm = tcb
	//c.log.Debug("tcb is %v", tcb)
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
		ksm, err := cbor.Marshal(ss, cbor.CanonicalEncOptions())
		if err != nil {
			return err
		}
		m[SCBlockContentSSigKey] = ksm
	}
	if ksok {
		c.sm[SCKeySignatureKey] = ks
		ksm, err := cbor.Marshal(ks, cbor.CanonicalEncOptions())
		if err != nil {
			return err
		}
		m[SCBlockContentPSigKey] = ksm
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

func (c *Contract) GetHashSig(did string) (string, string, string, error) {
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

	ss := util.GetStringFromMap(ssi, did)
	ks := util.GetStringFromMap(ksi, did)
	// ss == "" ||
	if ks == "" {
		return "", "", "", fmt.Errorf("invalid smart contract, share/key signature block is missing")
	}
	return hi.(string), ss, ks, nil
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

func (c *Contract) GetTotalRBTs() float64 {
	return util.GetFloatFromMap(c.sm, SCTotalRBTsKey)
}

func (c *Contract) GetPledgeMode() int {
	mi, ok := c.sm[SCPledgeModeKey]
	// Default mode is POW
	if !ok {
		return PeriodicPledgeMode
	}
	return mi.(int)
}

func (c *Contract) GetSenderDID() string {
	return c.getTransInfoString(TSSenderDIDKey)
}

func (c *Contract) GetReceiverDID() string {
	return c.getTransInfoString(TSReceiverDIDKey)
}

func (c *Contract) GetDeployerDID() string {
	return c.getTransInfoString(TSDeployerDIDKey)
}

func (c *Contract) GetExecutorDID() string {
	return c.getTransInfoString(TSExecutorDIDKey)
}

func (c *Contract) GetSmartContractData() string {
	return c.getTransInfoString(TSSmartContractDataKey)
}

func (c *Contract) GetComment() string {
	return c.getTransInfoString(TSCommentKey)
}

func (c *Contract) GetPinningServiceDID() string {
	return c.getTransInfoString(TSPinningDIDKey)
}

func (c *Contract) GetTransTokenInfo() []TokenInfo {
	tim := util.GetFromMap(c.sm, SCTransInfoKey)
	if tim == nil {
		return nil
	}
	tsm := util.GetFromMap(tim, TSTransInfoKey)
	if tsm == nil {
		return nil
	}
	ti := make([]TokenInfo, 0)
	tsmi, ok := tsm.(map[string]interface{})
	if ok {
		for k, v := range tsmi {
			t := TokenInfo{
				Token:      k,
				TokenType:  util.GetIntFromMap(v, TITokenTypeKey),
				OwnerDID:   util.GetStringFromMap(v, TIOwnerDIDKey),
				BlockID:    util.GetStringFromMap(v, TIBlockIDKey),
				TokenValue: util.GetFloatFromMap(v, TITokenValueKey),
			}
			ti = append(ti, t)
		}
	} else {
		tsmi, ok := tsm.(map[interface{}]interface{})
		if ok {
			for k, v := range tsmi {
				t := TokenInfo{
					Token:      util.GetString(k),
					TokenType:  util.GetIntFromMap(v, TITokenTypeKey),
					OwnerDID:   util.GetStringFromMap(v, TIOwnerDIDKey),
					BlockID:    util.GetStringFromMap(v, TIBlockIDKey),
					TokenValue: util.GetFloatFromMap(v, TITokenValueKey),
				}
				ti = append(ti, t)
			}
		} else {
			return nil
		}
	}
	return ti

}

func (c *Contract) GetCommitedTokensInfo() []TokenInfo {
	tim := util.GetFromMap(c.sm, SCTransInfoKey)
	if tim == nil {
		return nil
	}
	tsm := util.GetFromMap(tim, TSCommitedTokenInfoKey)
	if tsm == nil {
		return nil
	}
	ti := make([]TokenInfo, 0)
	tsmi, ok := tsm.(map[string]interface{})
	if ok {
		for k, v := range tsmi {
			t := TokenInfo{
				Token:      k,
				TokenType:  util.GetIntFromMap(v, TITokenTypeKey),
				OwnerDID:   util.GetStringFromMap(v, TIOwnerDIDKey),
				BlockID:    util.GetStringFromMap(v, TIBlockIDKey),
				TokenValue: util.GetFloatFromMap(v, TITokenValueKey),
			}
			ti = append(ti, t)
		}
	} else {
		tsmi, ok := tsm.(map[interface{}]interface{})
		if ok {
			for k, v := range tsmi {
				t := TokenInfo{
					Token:      util.GetString(k),
					TokenType:  util.GetIntFromMap(v, TITokenTypeKey),
					OwnerDID:   util.GetStringFromMap(v, TIOwnerDIDKey),
					BlockID:    util.GetStringFromMap(v, TIBlockIDKey),
					TokenValue: util.GetFloatFromMap(v, TITokenValueKey),
				}
				ti = append(ti, t)
			}
		} else {
			return nil
		}
	}
	return ti

}

func (c *Contract) UpdateSignature(dc did.DIDCrypto) error {
	did := dc.GetDID()
	hash, err := c.GetHash()
	if err != nil {
		return fmt.Errorf("Failed to get hash of smart contract, " + err.Error())
	}
	ssig, psig, err := dc.Sign(hash)
	if err != nil {
		return fmt.Errorf("Failed to get signature, " + err.Error())
	}

	if c.sm[SCShareSignatureKey] == nil {
		ksm := make(map[string]interface{})
		ksm[did] = util.HexToStr(ssig)
		c.sm[SCShareSignatureKey] = ksm
	} else {
		ksm, ok := c.sm[SCShareSignatureKey].(map[string]interface{})
		if ok {
			ksm[did] = util.HexToStr(ssig)
			c.sm[SCShareSignatureKey] = ksm
		} else {
			ksm, ok := c.sm[SCShareSignatureKey].(map[interface{}]interface{})
			if ok {
				ksm[did] = util.HexToStr(ssig)
				c.sm[SCShareSignatureKey] = ksm
			} else {
				return fmt.Errorf("failed to update signature, invalid share signature block")
			}
		}
	}
	if c.sm[SCKeySignatureKey] == nil {
		ksm := make(map[string]interface{})
		ksm[did] = util.HexToStr(psig)
		c.sm[SCKeySignatureKey] = ksm
	} else {
		ksm, ok := c.sm[SCKeySignatureKey].(map[string]interface{})
		if ok {
			ksm[did] = util.HexToStr(psig)
			c.sm[SCKeySignatureKey] = ksm
		} else {
			ksm, ok := c.sm[SCKeySignatureKey].(map[interface{}]interface{})
			if ok {
				ksm[did] = util.HexToStr(psig)
				c.sm[SCKeySignatureKey] = ksm
			} else {
				return fmt.Errorf("failed to update signature, invalid key signature block")
			}
		}
	}
	return c.blkEncode()
}

// This function is used by the quorums to verify sender's signature
func (c *Contract) VerifySignature(dc did.DIDCrypto) error {
	//fetch sender's did
	didstr := dc.GetDID()

	//fetch sender's signature
	hs, ss, ps, err := c.GetHashSig(didstr)
	if err != nil {
		c.log.Error("err", err)
		return err
	}

	//If the ss i.e., share signature is empty, then its a Pki sign, so call PvtVerify
	//Else it is NLSS based sign, so call NlssVerify
	didType := dc.GetSignType()
	if didType == did.BIPVersion {
		ok, err := dc.PvtVerify([]byte(hs), util.StrToHex(ps))

		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("did Pki signature verification failed")
		}
	} else {
		ok, err := dc.NlssVerify(hs, util.StrToHex(ss), util.StrToHex(ps))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("did Nlss signature verification failed")
		}
	}

	return nil
}
