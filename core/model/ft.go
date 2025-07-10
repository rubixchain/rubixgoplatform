package model

type CreateFTReq struct {
	DID             string  `json:"did"`
	FTName          string  `json:"ft_name"`
	FTCount         int     `json:"ft_count"`
	FTValue         float64 `json:"ft_value"`
	FTNumStartIndex int     `json:"ft_num_start_index"`
	FromRBT         bool    `json:"from_rbt"`
	IsHighValueFT   bool    `json:"is_high_value"`
}

type TransferFTReq struct {
	Receiver        string  `json:"receiver"`
	Sender          string  `json:"sender"`
	FTName          string  `json:"ft_name"`
	FTCount         int     `json:"ft_count"`
	Comment         string  `json:"comment"`
	QuorumType      int     `json:"quorum_type"`
	Password        string  `json:"password"`
	CreatorDID      string  `json:"creatorDID"`
	IsHighValueFT   bool    `json:"is_high_value"`
	FTTransferValue float64 `json:"ft_value"`
}

type GetFTInfo struct {
	BasicResponse
	FTInfo []FTInfo `json:"ft_info"`
}

type FTInfo struct {
	FTName      string `json:"ft_name"`
	FTCount     int    `json:"ft_count"`
	CreatorDID  string `json:"creator_did"`
	HighValueFT bool   `json:"high_value_ft"`
}

type FTInfoForExplorer struct {
	FTName     string `json:"ft_symbol"`
	FTCount    int    `json:"ft_balance"`
	CreatorDID string `json:"creator_did"`
}

type BurnFTReq struct {
	DID     string `json:"did"`
	FTName  string `json:"ft_name"`
	FTCount int    `json:"ft_count"`
	FromRBT bool   `json:"from_rbt"`
}
