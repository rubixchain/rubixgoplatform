package model

const (
	AlphaType int = iota
	BetaType
	GammaType
)

// QuorumListResponse used as model for the API responses
type QuorumListResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Result  []string `json:"result"`
}

type Quorum struct {
	Type    int    `json:"type"`
	Address string `json:"address"`
}

type QuorumList struct {
	Quorum []Quorum `json:"quorum"`
}

type CreditStatus struct {
	Score int `json:"score"`
}

type QuorumSetup struct {
	DID             string `json:"did"`
	Password        string `json:"password"`
	PrivKeyPassword string `json:"priv_password"`
}
