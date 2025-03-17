package block

// ----------GennesisBlock--------------------
// {
//   "1" : Type       : int
//   "2" : PreviousID : string
//   "3" : Info       : map[string]GenesisInfo
// }
// ----------GennesisInfo----------------------
// {
//   "1" : TokenLevel  : int
//   "2" : TokenNumber : string
//   "3" : MigratedID  : string
//   "4" : PreviousID  : string
// }

const (
	GenesisMigratedType int = iota
	GenesisPartType
	GenesisSmartContract
)

const (
	GBTypeKey string = "1"
	GBInfoKey string = "2"
)

const (
	GITokenLevelKey         string = "1"
	GITokenNumberKey        string = "2"
	GIMigratedBlkIDKey      string = "3"
	GIPreviousIDKey         string = "4"
	GIParentIDKey           string = "5"
	GIGrandParentIDKey      string = "6"
	GICommitedTokensKey     string = "7"
	GISmartContractValueKey string = "8"
)

type GenesisTokenInfo struct {
	Token              string        `json:"token"`
	TokenLevel         int           `json:"tokenLevel"`
	TokenNumber        int           `json:"tokenNumber"`
	MigratedBlockID    string        `json:"migratedBlockID"`
	PreviousID         string        `json:"previosuID"`
	ParentID           string        `json:"parentID"`
	GrandParentID      []string      `json:"grandParentID"`
	CommitedTokens     []TransTokens `json:"commitedTokens"`
	SmartContractValue float64       `json:"smartContractValue"`
}

type GenesisBlock struct {
	Type string             `json:"type"`
	Info []GenesisTokenInfo `json:"info"`
}

func newGenesisInfo(gi *GenesisTokenInfo) map[string]interface{} {
	ngib := make(map[string]interface{})
	ngib[GITokenLevelKey] = gi.TokenLevel
	ngib[GITokenNumberKey] = gi.TokenNumber
	if gi.MigratedBlockID != "" {
		ngib[GIMigratedBlkIDKey] = gi.MigratedBlockID
	}
	if gi.PreviousID != "" {
		ngib[GIPreviousIDKey] = gi.PreviousID
	}
	if gi.ParentID != "" {
		ngib[GIParentIDKey] = gi.ParentID
	}
	if gi.GrandParentID != nil {
		ngib[GIGrandParentIDKey] = gi.GrandParentID
	}
	//To add commited tokeninfo
	newCommitedTokensBlock := make(map[string]interface{})
	for _, tokensInfo := range gi.CommitedTokens {
		commitedTokenInfoMap := newTransToken(nil, &tokensInfo)
		if commitedTokenInfoMap == nil {
			return nil
		}
		newCommitedTokensBlock[tokensInfo.Token] = commitedTokenInfoMap
	}
	ngib[GICommitedTokensKey] = newCommitedTokensBlock
	if gi.SmartContractValue != 0 {
		ngib[GISmartContractValueKey] = gi.SmartContractValue
	}
	return ngib
}

func newGenesisBlock(gb *GenesisBlock) map[string]interface{} {
	if gb.Info == nil || len(gb.Info) == 0 {
		return nil
	}
	ngb := make(map[string]interface{})
	ngb[GBTypeKey] = gb.Type
	ngibs := make(map[string]interface{})
	for _, gi := range gb.Info {
		ngib := newGenesisInfo(&gi)
		if ngib == nil {
			return nil
		}
		ngibs[gi.Token] = ngib
	}
	ngb[GBInfoKey] = ngibs
	return ngb
}
