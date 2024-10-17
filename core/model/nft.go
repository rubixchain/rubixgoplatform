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
	Did          string  `json:"did"`
	ReceiverDid  string  `json:"receiverDid"`
	Type         int     `json:"type"`
	NFTBlockHash string  `json:"nftBlockHash"`
	NFTValue     float64 `json:"nftValue"`
}

type DeployNFTRequest struct {
	NFT        string
	DID        string
	QuorumType int
}

type ExecuteNFTRequest struct {
	NFT        string  `json:"nft"`
	Owner      string  `json:"owner"`
	Receiver   string  `json:"receiver"`
	QuorumType int     `json:"quorumType"`
	Comment    string  `json:"comment"`
	NFTValue   float64 `json:"nftValue"`
	NFTData    string  `json:"nftData"`
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
