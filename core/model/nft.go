package model

type NFTStatus struct {
	Token       string `json:"token"`
	TokenStatus int    `json:"token_status"`
}

type NFTTokens struct {
	BasicResponse
	Tokens []NFTStatus `json:"tokens"`
}
