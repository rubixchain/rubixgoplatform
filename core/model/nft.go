package model

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
	NFTMetadata  string  `json:"nftMetadata"`
	NFTFileName  string  `json:"nftFileName"`
}

type DeployNFTRequest struct {
	NFT         string  `json:"nft"`
	DID         string  `json:"did"`
	QuorumType  int     `json:"quorum_type"`
	NFTValue    float64 `json:"nft_value"`
	NFTData     string  `json:"nft_data"`
	NFTMetadata string  `json:"nft_metadata"`
	NFTFileName string  `json:"nft_file_name"`
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

type NFTInfo struct {
	NFTId    string  `json:"nft"`
	Owner    string  `json:"owner_did"`
	Value    float64 `json:"nft_value"`
	Metadata string  `json:"nft_metadata"`
	FileName string  `json:"nft_file_name"`
}

type NFTList struct {
	BasicResponse
	NFTs []NFTInfo `json:"nfts"`
}

type NFTInfoForExplorer struct {
	NFTId    string  `json:"nft_id"`
	NFTValue float64 `json:"nft_value"`
}
