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
	GBInfoKey string = "genesisInfo"
)

const (
	GIParentIDKey       string = "parentID"
	GICommitedTokensKey string = "commitedTokens"
	GINetworkIDKey          string = "networkID"
)

type GenesisTokenInfo struct {
	Token          string        `json:"token"`
	ParentID       string        `json:"parentID"`
	CommitedTokens []TransTokens `json:"commitedTokens"`
	NetworkID          string        `json:"networkID"`
}

type GenesisBlock struct {
	Info []GenesisTokenInfo `json:"info"`
}

func newGenesisInfo(gi *GenesisTokenInfo) map[string]interface{} {
	ngib := make(map[string]interface{})
	if gi.ParentID != "" {
		ngib[GIParentIDKey] = gi.ParentID
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

	if gi.NetworkID != "" {
		ngib[GINetworkIDKey] = gi.NetworkID
	}

	return ngib
}

func newGenesisBlock(gb *GenesisBlock) map[string]interface{} {
	if gb.Info == nil || len(gb.Info) == 0 {
		return nil
	}
	ngb := make(map[string]interface{})
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
