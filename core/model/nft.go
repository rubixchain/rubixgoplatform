package model

type NFTStatus struct {
	Token       string `json:"token"`
	TokenStatus int    `json:"token_status"`
}

type NFTTokens struct {
	BasicResponse
	Tokens []NFTStatus `json:"tokens"`
}

type NFTDeployEvent struct {
	NFT          string `json:"nft"`
	Did          string `json:"did"`
	Type         int    `json:"type"`
	NFTBlockHash string `json:"smartContractBlockHash"`
}
