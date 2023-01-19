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

type RBTInfo struct {
	BasicResponse
	WholeRBT        int `json:"whole_rbt"`
	PledgedWholeRBT int `json:"pledged_whole_rbt"`
	LockedWholeRBT  int `json:"locked_whole_rbt"`
	PartRBT         int `json:"part_rbt"`
	PledgedPartRBT  int `json:"pledged_part_rbt"`
	LockedPartRBT   int `json:"locked_part_rbt"`
}
