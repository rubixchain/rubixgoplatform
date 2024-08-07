package model

const (
	RBTType string = "RBT"
	DTType  string = "DT"
	NFTType string = "NFT"
)

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

type RBTPinRequest struct {
	PinningNode string  `json:"pinningNode"`
	Sender      string  `json:"sender"`
	TokenCount  float64 `json:"tokenCOunt"`
	Comment     string  `json:"comment"`
	Type        int     `json:"type"`
	Password    string  `json:"password"`
}

type RBTRecoverRequest struct {
	PinningNode string  `json:"pinningNode"`
	Sender      string  `json:"sender"`
	TokenCount  float64 `json:"tokenCount"`
	Password    string  `json:"password"`
}

type RBTTransferReply struct {
	BasicResponse
	Receiver   string `json:"receiver"`
	Sender     string `json:"sender"`
	TokenCount int    `json:"tokenCOunt"`
	Comment    string `json:"comment"`
	Type       int    `json:"type"`
}

type GetAccountInfo struct {
	BasicResponse
	AccountInfo []DIDAccountInfo `json:"account_info"`
}

type DIDAccountInfo struct {
	DID        string  `json:"did"`
	DIDType    int     `json:"did_type"`
	RBTAmount  float64 `json:"rbt_amount"`
	PledgedRBT float64 `json:"pledged_rbt"`
	LockedRBT  float64 `json:"locked_rbt"`
	PinnedRBT  float64 `json:"pinned_rbt"`
}

type TokenDetail struct {
	Token  string `json:"token"`
	Status int    `json:"status"`
}

type TokenResponse struct {
	BasicResponse
	TokenDetails []TokenDetail `json:"token_detials"`
}

type PledgedTokenStateDetails struct {
	DID            string `json:"did"`
	TokensPledged  string `json:"token"`
	TokenStateHash string `json:"token_state"`
}

type TokenStateResponse struct {
	BasicResponse
	PledgedTokenStateDetails []PledgedTokenStateDetails `json:"token_state_details"`
}
