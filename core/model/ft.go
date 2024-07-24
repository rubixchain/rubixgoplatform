package model

type CreateFTReq struct {
	DID        string  `json:"did"`
	FTName     string  `json:"ftname"`
	FTCount    int     `json:"ftcount"`
	TokenCount float64 `json:"tokencount"`
}
