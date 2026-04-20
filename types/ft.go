package types

import "github.com/rubixchain/rubixgoplatform/types/models"

type CreateFTReq struct {
	DID             string `json:"did"`
	FTName          string `json:"ft_name"`
	FTCount         int    `json:"ft_count"`
	TokenCount      int    `json:"token_count"`
	FTNumStartIndex int    `json:"ft_num_start_index"`
}

type FTMap struct {
	FTInfo models.FTInfo
	FTValue  float64
	FTIdList []string
}