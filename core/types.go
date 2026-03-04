package core

type Token struct {
	TokenHash  string  `json:"token_hash"`
	TokenValue float64 `json:"token_value"`
}

type AllToken struct {
	TokenHash   string `json:"tokenHash"`
	BlockHash   string `json:"blockHash"`
	BlockNumber int    `json:"blockNumber"`
}