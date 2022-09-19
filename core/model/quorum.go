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
