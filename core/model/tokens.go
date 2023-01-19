package model

type RBTGenerateRequest struct {
	NumberOfTokens int    `json:"number_of_tokens"`
	DID            string `json:"did"`
}

type RBTTransferRequest struct {
	Receiver   string  `json:"receiver"`
	Sender     string  `json:"sender"`
	TokenCount float64 `json:"tokenCOunt"`
	Comment    string  `json:"comment"`
	Type       int     `json:"type"`
	Password   string  `json:"password"`
}

type RBTTransferReply struct {
	BasicResponse
	Receiver   string `json:"receiver"`
	Sender     string `json:"sender"`
	TokenCount int    `json:"tokenCOunt"`
	Comment    string `json:"comment"`
	Type       int    `json:"type"`
}
