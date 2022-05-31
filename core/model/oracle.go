package model

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
