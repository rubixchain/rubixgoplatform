package block

import (
	"fmt"
	"math"
)

const (
	OTCTransTypeKey         string = "transactionType"
	OTCOwnerKey             string = "owner"
	OTCSenderDIDKey         string = "sender"
	OTCReceiverDIDKey       string = "receiver"
	OTCCommentKey           string = "comment"
	OTCTIDKey               string = "tid"
	OTCGroupKey             string = "group"
	OTCWholeTokensKey       string = "wholeTokens"
	OTCWholeTokensIDKey     string = "wholeTokensID"
	OTCPartTokensKey        string = "partTokens"
	OTCPartTokensIDKey      string = "partTokensID"
	OTCQuorumSignatureKey   string = "quorumSignature"
	OTCPledgeTokenKey       string = "pledgeToken"
	OTCTokensPledgedForKey  string = "tokensPledgedFor"
	OTCTokensPledgedWithKey string = "tokensPledgedWith"
	OTCTokensPledgeMapKey   string = "tokensPledgeMap"
	OTCDistributedObjectKey string = "distributedObject"
	OTCBlockHashKey         string = "hash"
	OTCSignatureKey         string = "signature"
	OTCSenderSignKey        string = "senderSign"
	OTCPvtShareKey          string = "pvtShareBits"
	OTCTokenChainBlockKey   string = "tokenChainBlock"
	OTCMinIDKey             string = "mineID"
	OTCStakedTokenKey       string = "stakedToken"
)

var OTCKey = []string{OTCTransTypeKey, OTCOwnerKey, OTCTokensPledgedWithKey, OTCTokensPledgedForKey, OTCReceiverDIDKey, OTCSenderDIDKey, OTCCommentKey, OTCTokensPledgeMapKey, OTCDistributedObjectKey, OTCTIDKey, OTCPledgeTokenKey, OTCGroupKey, OTCWholeTokensKey, OTCWholeTokensIDKey, OTCPartTokensKey, OTCPartTokensIDKey, OTCQuorumSignatureKey}

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
					if k == OTCTokensPledgedWithKey {
						str, err = tcMarshal(str, v, []string{"node", "tokens"})
					} else if k == OTCTokenChainBlockKey {
						str, err = tcMarshal(str, v, OTCKey)
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

// Rounds off float to MaxDecimalPlaces
// TODO: this function is taken from core package ( floatPrecision() ) because of cyclic dependency issue
// Later, it needs to added in a seperate package.
func floatPrecisionToMaxDecimalPlaces(num float64) float64 {
	precision := 3 // Taken from MaxDecimalPlaces of core package
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}
