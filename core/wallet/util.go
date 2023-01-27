package wallet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/util"
	"golang.org/x/crypto/sha3"
)

var TCKey = []string{TCTransTypeKey, TCOwnerKey, TCTokensPledgedWithKey, TCTokensPledgedForKey, TCReceiverDIDKey, TCSenderDIDKey, TCCommentKey, TCTokensPledgeMapKey, TCDistributedObjectKey, TCTIDKey, TCPledgeTokenKey, TCGroupKey, TCWholeTokensKey, TCWholeTokensIDKey, TCPartTokensKey, TCPartTokensIDKey, TCQuorumSignatureKey, TCTokenChainBlockKey, TCPreviousHashKey, TCNonceKey, TCBlockNumber}

func GetBlockID(t string, tc map[string]interface{}) (string, error) {
	ha, ok := tc[TCBlockHashKey]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block hash")
	}
	bnmi, ok := tc[TCBlockNumber]
	if !ok {
		return "", fmt.Errorf("invalid token chain block, missing block number")
	}
	bnm := bnmi.([]interface{})
	for _, s := range bnm {
		ss := strings.Split(s.(string), " ")
		if ss[0] == t {
			return ha.(string) + "-" + ss[1], nil
		}
	}
	return "", fmt.Errorf("invalid token chain block, missing block number")
}

func GetBlockNumber(t string, tc map[string]interface{}) (uint64, error) {
	if tc == nil {
		return 0xFFFFFFFFFFFFFFFF, nil
	}
	bnmi, ok := tc[TCBlockNumber]
	if !ok {
		return 0, fmt.Errorf("invalid token chain block, missing block number")
	}

	bnm := bnmi.([]interface{})
	for _, s := range bnm {
		ss := strings.Split(s.(string), " ")
		if ss[0] == t {
			num, err := strconv.ParseUint(ss[1], 10, 64)
			if err != nil {
				return 0, err
			}
			return num, nil
		}
	}
	return 0, fmt.Errorf("invalid token chain block, missing block number")

}

func racMarshal(str string, m interface{}, keys []string) (string, error) {
	var err error
	switch mt := m.(type) {
	case map[string]interface{}:
		str = str + "{"
		c1 := false
		if keys == nil {
			for k, v := range mt {
				if c1 {
					str = str + ","
				}
				c1 = true
				str = str + "\"" + k + "\":"
				str, err = racMarshal(str, v, keys)
				if err != nil {
					return "", err
				}
			}
		} else {
			for _, k := range keys {
				v, ok := mt[k]
				if ok {
					if c1 {
						str = str + ","
					}
					c1 = true
					str = str + "\"" + k + "\":"
					str, err = racMarshal(str, v, keys)
					if err != nil {
						return "", err
					}
				}
			}
		}
		str = str + "}"
	case string:
		str = str + "\"" + mt + "\""
	case int:
		str = str + fmt.Sprintf("%d", mt)
	default:
		return "", fmt.Errorf("invalid type %T", mt)
	}
	return str, nil
}

func RAC2Hash(rac map[string]interface{}) ([]byte, error) {
	if rac == nil {
		return nil, fmt.Errorf("invalid RAC")
	}
	_, ok := rac["pvtKeySign"]
	if ok {
		delete(rac, "pvtKeySign")
	}
	keys := []string{RACTypeKey, RACDidKey, RACTotalSupplyKey, RACTokenCountKey, RACCreatorInputKey, RACHashKey, RACUrlKey, RACVersionKey, RACNonceKey}
	var err error
	str := ""
	str, err = racMarshal(str, rac, keys)
	if err != nil {
		return nil, err
	}
	h := sha3.New256()
	h.Write([]byte(str))
	b := h.Sum(nil)
	return b, nil
}

func tcMarshal(str string, m interface{}, keys []string) (string, error) {
	var err error
	switch mt := m.(type) {
	case []map[string]interface{}:
		str = str + "["
		c1 := false
		for i := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str, err = tcMarshal(str, mt[i], keys)
			if err != nil {
				return "", err
			}
		}
		str = str + "]"
	case map[string]interface{}:
		str = str + "{"
		c1 := false
		if keys == nil {
			for k, v := range mt {
				if c1 {
					str = str + ","
				}
				c1 = true
				str = str + "\"" + k + "\":"
				s, ok := v.(string)
				if ok {
					str = str + "\"" + s + "\""
				} else {
					str, err = tcMarshal(str, v, keys)
					if err != nil {
						return "", err
					}
				}
			}
		} else {
			for _, k := range keys {
				v, ok := mt[k]
				if ok {
					if c1 {
						str = str + ","
					}
					c1 = true
					str = str + "\"" + k + "\":"
					if k == TCTokensPledgedWithKey {
						str, err = tcMarshal(str, v, []string{"node", "tokens"})
					} else if k == TCTokenChainBlockKey {
						str, err = tcMarshal(str, v, TCKey)
					} else {
						str, err = tcMarshal(str, v, nil)
					}
					if err != nil {
						return "", err
					}
				}
			}
		}
		str = str + "}"
	case map[string]string:
		str = str + "{"
		c1 := false
		if keys == nil {
			for k, v := range mt {
				if c1 {
					str = str + ","
				}
				c1 = true
				str = str + "\"" + k + "\":"
				str = str + "\"" + v + "\""
			}
		} else {
			for _, k := range keys {
				v, ok := mt[k]
				if ok {
					if c1 {
						str = str + ","
					}
					c1 = true
					str = str + "\"" + k + "\":"
					str = str + "\"" + v + "\""
				}
			}
		}
		str = str + "}"
	case []string:
		str = str + "["
		c1 := false
		for _, mf := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str, err = tcMarshal(str, mf, nil)
			if err != nil {
				return "", err
			}
		}
		str = str + "]"
	case string:
		str = str + "\"" + mt + "\""
	case []interface{}:
		str = str + "["
		c1 := false
		for _, mf := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			s, ok := mf.(string)
			if ok {
				str = str + "\"" + s + "\""
			} else {
				str, err = tcMarshal(str, mf, keys)
				if err != nil {
					return "", err
				}
			}
		}
		str = str + "]"
	default:
		return "", fmt.Errorf("invalid type %T", mt)
	}
	return str, nil
}

func TC2HashString(tc map[string]interface{}) (string, error) {
	_, ok := tc[TCBlockHashKey]
	if ok {
		delete(tc, TCBlockHashKey)
	}
	_, ok = tc[TCSignatureKey]
	if ok {
		delete(tc, TCSignatureKey)
	}
	var err error
	str := ""
	str, err = tcMarshal(str, tc, TCKey)
	if err != nil {
		return "", err
	}
	return util.CalculateHashString(str, "SHA3-256"), nil
}

func CreateTCBlock(ctcb map[string]interface{}, tcb *TokenChainBlock) map[string]interface{} {
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
	if len(tcb.TokensPledgeMap) != 0 {
		ntcb[TCTokensPledgeMapKey] = tcb.TokensPledgeMap
	}
	if tcb.TokenChainDetials != nil {
		ntcb[TCTokenChainBlockKey] = tcb.TokenChainDetials
	}
	if ctcb == nil {
		return nil
	}
	phm := make([]interface{}, 0)
	bnm := make([]interface{}, 0)
	for t, tc := range ctcb {
		if tc == nil {
			bnm = append(bnm, t+" 0")
		} else {
			mb, ok := tc.(map[string]interface{})
			if !ok {
				return nil
			}
			bn, err := GetBlockNumber(t, mb)
			if err != nil {
				return nil
			}
			bn++
			ph, ok := mb[TCBlockHashKey]
			if !ok {
				return nil
			}
			phm = append(phm, t+" "+ph.(string))
			bnm = append(bnm, t+" "+strconv.FormatUint(bn, 10))
		}
	}
	ntcb[TCBlockNumber] = bnm
	ntcb[TCPreviousHashKey] = phm
	h, err := TC2HashString(ntcb)
	if err != nil {
		return nil
	}
	ntcb[TCBlockHashKey] = h
	return ntcb
}

func GetTCHashSig(tcb map[string]interface{}) (string, string, error) {
	h, ok := tcb[TCBlockHashKey]
	if !ok {
		return "", "", fmt.Errorf("Invalid token chain block, missing block hash")
	}
	s, ok := tcb[TCSignatureKey]
	if !ok {
		return "", "", fmt.Errorf("Invalid token chain block, missing block signature")
	}
	return h.(string), s.(string), nil
}

func GetTCHash(tcb map[string]interface{}) (string, error) {
	h, ok := tcb[TCBlockHashKey]
	if !ok {
		return "", fmt.Errorf("Invalid token chain block, missing block hash")
	}
	return h.(string), nil
}

func GetTCTransType(tcb map[string]interface{}) string {
	tt, ok := tcb[TCTransTypeKey]
	if !ok {
		return ""
	}
	return tt.(string)
}

func GetTCSenderDID(tcb map[string]interface{}) string {
	tt, ok := tcb[TCSenderDIDKey]
	if !ok {
		return ""
	}
	return tt.(string)
}

func GetTCReceiverDID(tcb map[string]interface{}) string {
	tt, ok := tcb[TCReceiverDIDKey]
	if !ok {
		return ""
	}
	return tt.(string)
}
