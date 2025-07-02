package model

type CreateFTReq struct {
	DID             string `json:"did"`
	FTName          string `json:"ft_name"`
	FTCount         int    `json:"ft_count"`
	TokenCount      int    `json:"token_count"`
	FTNumStartIndex int    `json:"ft_num_start_index"`
}

type TransferFTReq struct {
	Receiver   string `json:"receiver"`
	Sender     string `json:"sender"`
	FTName     string `json:"ft_name"`
	FTCount    int    `json:"ft_count"`
	Comment    string `json:"comment"`
	QuorumType int    `json:"quorum_type"`
	Password   string `json:"password"`
	CreatorDID string `json:"creatorDID"`
}

type GetFTInfo struct {
	BasicResponse
	FTInfo []FTInfo `json:"ft_info"`
}

type FTInfo struct {
	FTName     string `json:"ft_name"`
	FTCount    int    `json:"ft_count"`
	CreatorDID string `json:"creator_did"`
}

type FTInfoForExplorer struct {
	FTName     string `json:"ft_symbol"`
	FTCount    int    `json:"ft_balance"`
	CreatorDID string `json:"creator_did"`
}
