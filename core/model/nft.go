package model

type NFTReq struct {
	DID        string
	NumTokens  int
	Fields     map[string][]string
	FileNames  []string
	FolderName string
}

type CreateNFTReq struct {
	DID      string
	UserID   string
	UserInfo string
	FileInfo string
	Files    []string
}

type NFTStatus struct {
	Token       string `json:"token"`
	TokenStatus int    `json:"token_status"`
}

type NFTTokens struct {
	BasicResponse
	Tokens []NFTStatus `json:"tokens"`
}

type NFTEvent struct {
	NFT          string  `json:"nft"`
	ExecutorDid  string  `json:"executorDid"`
	ReceiverDid  string  `json:"receiverDid"`
	Type         int     `json:"type"`
	NFTBlockHash string  `json:"nftBlockHash"`
	NFTValue     float64 `json:"nftValue"`
}

type DeployNFTRequest struct {
	NFT        string  `json:"nft"`
	DID        string  `json:"did"`
	QuorumType int     `json:"quorum_type"`
	NFTValue   float64 `json:"nft_value"`
	NFTData    string  `json:"nft_data"`
}

type ExecuteNFTRequest struct {
	NFT        string  `json:"nft"`
	Executor   string  `json:"executor"`
	Receiver   string  `json:"receiver"`
	QuorumType int     `json:"quorum_type"`
	Comment    string  `json:"comment"`
	NFTValue   float64 `json:"nft_value"`
	NFTData    string  `json:"nft_data"`
}

type NewNFTSubscription struct {
	NFT string `json:"nft"`
}

type NewNFTEvent struct {
	NFT          string `json:"nft"`
	OwnerDid     string `json:"ownerDid"`
	ReceiverDid  string `json:"receiverDid"`
	Type         int    `json:"type"`
	NFTBlockHash string `json:"nftBlockHash"`
}

type NFTInfo struct {
	NFTId string  `json:"nft"`
	Owner string  `json:"owner_did"`
	Value float64 `json:"nft_value"`
}

type NFTList struct {
	BasicResponse
	NFTs []NFTInfo `json:"nfts"`
}
