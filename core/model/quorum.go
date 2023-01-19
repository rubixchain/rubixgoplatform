package model

const (
	AlphaType int = iota
	BetaType
	GammaType
)

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
	DID      string `json:"did"`
	Password string `json:"password"`
}
