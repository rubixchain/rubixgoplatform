package model

const (
	AlphaType int = iota
	BetaType
	GammaType
)

// QuorumListResponse used as model for the API responses
type QuorumListResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Result  []string `json:"result"`
}

type Quorum struct {
	Type    int    `json:"type"`
	Address string `json:"address"`
}

type QuorumList struct {
	Quorum []Quorum `json:"quorum"`
}

type CreditStatus struct {
	Score int `json:"score"`
}

type QuorumSetup struct {
	DID             string `json:"did"`
	Password        string `json:"password"`
	PrivKeyPassword string `json:"priv_password"`
}

type AddUnpledgeDetailsRequest struct {
	TransactionHash   string   `json:"transaction_hash"`
	QuorumDID         string   `json:"quorum_did"`
	PledgeTokenHashes []string `json:"pledge_token_hashes"`
	TransactionEpoch  int64    `json:"transaction_epoch"`
}

type InitiateUnpledgeResponse struct {
	BasicResponse
}

type CheckAllTokenOwnershipRequest struct {
	TransTokens []string `json:"trans_tokens"`
	OwnerDID    string   `json:"owner_did"`
}

type CheckAllTokenOwnershipResponse struct {
	BasicResponse
	AllTokensOwned bool `json:"all_tokens_owned"`
}
