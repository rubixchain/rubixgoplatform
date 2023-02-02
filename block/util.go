package block

import (
	"fmt"
)

var TCKey = []string{TCTransTypeKey, TCOwnerKey, TCTokensPledgedWithKey, TCTokensPledgedForKey, TCReceiverDIDKey, TCSenderDIDKey, TCCommentKey, TCTokensPledgeMapKey, TCDistributedObjectKey, TCTIDKey, TCPledgeTokenKey, TCGroupKey, TCWholeTokensKey, TCWholeTokensIDKey, TCPartTokensKey, TCPartTokensIDKey, TCQuorumSignatureKey}

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
