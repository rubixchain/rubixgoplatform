package wallet

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/util"
	"golang.org/x/crypto/sha3"
)

var TCKey = []string{TCTransTypeKey, TCOwnerKey, TCTokensPledgedWithKey, TCTokensPledgedForKey, TCReceiverDIDKey, TCSenderDIDKey, TCCommentKey, TCDistributedObjectKey, TCTIDKey, TCPledgeTokenKey, TCGroupKey, TCTokenChainBlockKey, TCPreviousHashKey, TCNonceKey}

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
