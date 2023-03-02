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
// }

const (
	GenesisMigratedType int = iota
)

const (
	GBTypeKey string = "1"
	GBInfoKey string = "2"
)

const (
	GITokenLevelKey    string = "1"
	GITokenNumberKey   string = "2"
	GIMigratedBlkIDKey string = "3"
)

type GenesisTokenInfo struct {
	Token           string `json:"token"`
	TokenLevel      int    `json:"tokenLevel"`
	TokenNumber     int    `json:"tokenNumber"`
	MigratedBlockID string `json:"migratedBlockID"`
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
