package model

// type Input struct {
// 	Server             string        `json:"server"`
// 	Function           string        `json:"function"`
// 	AddInput           []NodeID      `json:"addInput"`
// 	AssignCreditsInput AssignCredits `json:"assignCredits"`
// 	UpdateMineInput    UpdateMine    `json:"updateMine"`
// 	GetQuorumInput     GetQuorum     `json:"getQuorum"`
// 	UpdateQuorumInput  UpdateQuorum  `json:"updateQuorum"`
// }

type Input struct {
	Server   string      `json:"server"`
	Function string      `json:"function"`
	Input    interface{} `json:"input"`
}

type TokenID struct {
	Token int `json:"token"`
	Level int `json:"level"`
}

type NodeID struct {
	PeerID     string `json:"peerid"`
	DIDHash    string `json:"didHash"`
	WalletHash string `json:"walletHash"`
}

type UpdateMine struct {
	DIDHash string `json:"didhash"`
	Credits int    `json:"credits"`
}

type AssignCredits struct {
	DIDHash string `json:"didHash"`
	Credits int    `json:"credits"`
}

type GetQuorum struct {
	Receiver   string `json:"receiver"`
	TokenCount int    `json:"tokencount"`
	Sender     string `json:"sender"`
	BetaHash   string `json:"betahash"`
	GammaHash  string `json:"gammahash"`
}

type UpdateQuorum struct {
	CompleteQuorum []string `json:"completequorum"`
	SignedQuorum   []string `json:"signedquorum"`
	Status         bool     `json:"status"`
}
