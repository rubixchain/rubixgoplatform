package contract

import "github.com/rubixchain/rubixgoplatform/util"

// ----------TransInfo----------------------
// {
// 	 "1" : SenderDID      : string
// 	 "2" : ReceiverDID    : string
// 	 "3" : Comment        : string
// 	 "4" : TransTokens    : TokenInfo
// 	 "5" : ExchangeTokens : TokenInfo
// }

// ----------TokenInfo----------------------
// {
// 	 "1" : TokenType      : int
// 	 "2" : OwnerDID       : string
// 	 "3" : BlockID        : string
// }

const (
	TSSenderDIDKey          string = "1"
	TSReceiverDIDKey        string = "2"
	TSCommentKey            string = "3"
	TSTransInfoKey          string = "4"
	TSExcahngeInfoKey       string = "5"
	TSDeployerDIDKey        string = "6"
	TSSmartContractTokenKey string = "7"
	TSCommitedTokenInfoKey  string = "8"
	TSExecutorDIDKey        string = "9"
	TSSmartContractDataKey  string = "10"
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
	SenderDID          string      `json:"senderDID"`
	ReceiverDID        string      `json:"receiverDID"`
	Comment            string      `json:"comment"`
	TransTokens        []TokenInfo `json:"TransTokens"`
	ExchangeTokens     []TokenInfo `json:"excahngeTokens"`
	CommitedTokens     []TokenInfo `json:"comitedtokens"`
	DeployerDID        string      `json:"deployerDID`
	SmartContractToken string      `json:"smartcontractToken`
	ExecutorDID        string      `json:"executorDID"`
	SmartContractData  string      `json:"smartcontractdata"`
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
	if ts.DeployerDID != "" {
		ntsb[TSDeployerDIDKey] = ts.DeployerDID
	}
	if ts.ExecutorDID != "" {
		ntsb[TSExecutorDIDKey] = ts.ExecutorDID
	}
	if ts.SmartContractToken != "" {
		ntsb[TSSmartContractTokenKey] = ts.SmartContractToken
	}
	if ts.Comment != "" {
		ntsb[TSCommentKey] = ts.Comment
	}
	if ts.SmartContractData != "" {
		ntsb[TSSmartContractDataKey] = ts.SmartContractData
	}

	if ts.CommitedTokens != nil && len(ts.CommitedTokens) > 0 {
		ntibs := make(map[string]interface{})
		for _, ti := range ts.CommitedTokens {
			ntib := newTokenInfoBlock(&ti)
			if ntib == nil {
				return nil
			}
			ntibs[ti.Token] = ntib
		}
		ntsb[TSCommitedTokenInfoKey] = ntibs
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
		ntsb[TSExcahngeInfoKey] = ntibs
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
