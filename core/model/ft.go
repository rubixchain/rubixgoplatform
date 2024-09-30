package model

type CreateFTReq struct {
	DID        string  `json:"did"`
	FTName     string  `json:"ftname"`
	FTCount    int     `json:"ftcount"`
	TokenCount float64 `json:"tokencount"`
}

type TransferFTReq struct {
	Receiver string `json:"receiver"`
	Sender   string `json:"sender"`
	FTName   string `json:"FTName"`
	FTCount  int    `json:"FTCount"`
	Comment  string `json:"comment"`
	Type     int    `json:"type"`
	Password string `json:"password"`
}

type GetFTInfo struct {
	BasicResponse
	FTInfo []FTInfo `json:"ft_info"`
}

type FTInfo struct {
	FTName  string `json:"ftname"`
	FTCount int    `json:"ft_count"`
}
