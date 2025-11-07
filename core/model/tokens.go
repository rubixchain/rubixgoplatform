package model

const (
	RBTType string = "RBT"
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

type FaucetRBTGenerateRequest struct {
	TokenCount int    `json:"token_count"`
	DID        string `json:"did"`
}

type TokenProviderMap struct {
	Token         string  `gorm:"column:token;primaryKey"`
	DID           string  `gorm:"column:did"`
	FuncID        int     `gorm:"column:func_id"`
	Role          int     `gorm:"column:role"`
	TransactionID string  `gorm:"column:transaction_id"`
	Sender        string  `gorm:"column:sender"`
	Receiver      string  `gorm:"column:receiver"`
	TokenValue    float64 `gorm:"column:token_value"`
}

type PinCheckReply struct {
	Status     bool              `json:"status"`
	Message    string            `json:"message"`
	PinDetails *TokenProviderMap `json:"pin_details"`
}

type SendTokenDetailsInfo struct {
	TokenChainLength uint64 `json:"tc_length"`
	TokenType        int    `json:"token_type"`
	Token            string `json:"token"`
	Did              string `json:"did"`
	AssetType        int    `json:"asset_type"`
}
type FailedToSyncTokenDetailsInfo struct {
	TokenID   string `gorm:"column:token_id;primaryKey"` // `gorm:"column:token;primaryKey"`
	TokenType int    `gorm:"column:token_type"`
	Did       string `gorm:"column:did"`
	AssetType int    `gorm:"column:asset_type"`
}

type FullNodeTxnHistoryInfo struct {
	TransactionID    string  `gorm:"column:transaction_id;primaryKey"`
	TransactionValue float64 `gorm:"column:transaction_value"`
	BlockHash        string  `gorm:"column:block_hash"`
}

type DoubleSpentTokenInfo struct {
	TokenID        string `gorm:"column:token_id;primaryKey"` // `gorm:"column:token;primaryKey"`
	AssetType      int    `gorm:"column:asset_type"`
	TokenType      int    `gorm:"column:token_type"`
	PublisherDID   string `gorm:"column:publisher_did"`
	ClaimedOwnerI  string `gorm:"column:claimed_owner_I"`
	ClaimedOwnerII string `gorm:"column:claimed_owner_II"`
	ErrorMessage   string `gorm:"column:error"`
}

type TokenChainDetailsEvent struct {
	PublisherPeerID string                 `json:"publisher_peer_id"`
	TokenDetails    []SendTokenDetailsInfo `json:"token_details"`
	BatchNumber     int                    `json:"batch_number"`
}
