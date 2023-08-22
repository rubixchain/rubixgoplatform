package contract

import "github.com/rubixchain/rubixgoplatform/util"

// ----------TransInfo----------------------
// {
// 	 "1" : SenderDID        : string
// 	 "2" : ReceiverDID      : string
// 	 "3" : Comment          : string
// 	 "4" : TransTokens      : map[string]TokenInfo
// 	 "5" : ExchangeTokens   : map[string]TokenInfo
//   "6" : BatchTransTokens : ma[string]map[string]TokenInfo
// }

// ----------TokenInfo----------------------
// {
// 	 "1" : TokenType      : int
// 	 "2" : OwnerDID       : string
// 	 "3" : BlockID        : string
// }

const (
	TSSenderDIDKey      string = "1"
	TSReceiverDIDKey    string = "2"
	TSCommentKey        string = "3"
	TSTransInfoKey      string = "4"
	TSExchangeInfoKey   string = "5"
	TSBatchTransInfoKey string = "6"
)

const (
	TITokenTypeKey  string = "1"
	TIOwnerDIDKey   string = "2"
	TIBlockIDKey    string = "3"
	TITokenValueKey string = "4"
)

type TokenInfo struct {
	Token      string  `json:"token"`
	TokenType  int     `json:"tokenType"`
	TokenValue float64 `json:"tokenValue"`
	OwnerDID   string  `json:"ownerDID"`
	BlockID    string  `json:"blockID"`
}

type TransInfo struct {
	SenderDID        string                 `json:"senderDID"`
	ReceiverDID      string                 `json:"receiverDID"`
	Comment          string                 `json:"comment"`
	TransTokens      []TokenInfo            `json:"transTokens"`
	ExchangeTokens   []TokenInfo            `json:"exchangeTokens"`
	BatchTransTokens map[string][]TokenInfo `json:"batchTransTokens"`
}

func newTokenInfoBlock(ti *TokenInfo) map[string]interface{} {
	ntib := make(map[string]interface{})
	ntib[TITokenTypeKey] = ti.TokenType
	ntib[TITokenValueKey] = ti.TokenValue
	if ti.OwnerDID != "" {
		ntib[TIOwnerDIDKey] = ti.OwnerDID
	}
	if ti.BlockID != "" {
		ntib[TIBlockIDKey] = ti.BlockID
	}
	return ntib
}

func newTransInfoBlock(ts *TransInfo) map[string]interface{} {
	ntsb := make(map[string]interface{})
	if ts.SenderDID != "" {
		ntsb[TSSenderDIDKey] = ts.SenderDID
	}
	if ts.ReceiverDID != "" {
		ntsb[TSReceiverDIDKey] = ts.ReceiverDID
	}
	if ts.Comment != "" {
		ntsb[TSCommentKey] = ts.Comment
	}
	if ts.TransTokens != nil && len(ts.TransTokens) > 0 {
		ntibs := make(map[string]interface{})
		for _, ti := range ts.TransTokens {
			ntib := newTokenInfoBlock(&ti)
			if ntib == nil {
				return nil
			}
			ntibs[ti.Token] = ntib
		}
		ntsb[TSTransInfoKey] = ntibs
	} else if ts.BatchTransTokens != nil {
		nbtibs := make(map[string]map[string]interface{})
		for k, v := range ts.BatchTransTokens {
			ntibs := make(map[string]interface{})
			for _, ti := range v {
				ntib := newTokenInfoBlock(&ti)
				if ntib == nil {
					return nil
				}
				ntibs[ti.Token] = ntib
			}
			nbtibs[k] = ntibs
		}
		ntsb[TSBatchTransInfoKey] = nbtibs
	}
	if ts.ExchangeTokens != nil && len(ts.ExchangeTokens) > 0 {
		ntibs := make(map[string]interface{})
		for _, ti := range ts.ExchangeTokens {
			ntib := newTokenInfoBlock(&ti)
			if ntib == nil {
				return nil
			}
			ntibs[ti.Token] = ntib
		}
		ntsb[TSExchangeInfoKey] = ntibs
	}
	return ntsb
}

func (c *Contract) getTransInfoString(key string) string {
	tim := util.GetFromMap(c.sm, SCTransInfoKey)
	if tim == nil {
		return ""
	}
	return util.GetStringFromMap(tim, key)
}
