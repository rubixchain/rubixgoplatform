package model

type MiningRequest struct {
	MinerDid           string          `json:"miner"`
	TokenCredits       uint64          `json:"credits"`
	MiningTokenDetails NewTokenDetails `json:"token_details"`
	Password           string          `json:"password"`
	Type               int             `json:"type"`
}

type NewTokenDetails struct {
	TokenLevel  int    `json:"token_level"`
	TokenNumber uint64 `json:"token_number"`
}

type PledgeHistory struct {
	QuorumDID          string  `gorm:"column:quorum_did"`
	TransactionID      string  `gorm:"column:transaction_id"`
	TransactionType    int     `gorm:"column:transaction_type"`
	TransferTokenID    string  `gorm:"column:transfer_tokens_id"`
	TransferTokenType  int     `gorm:"column:transfer_tokens_type"`
	TransferTokenValue float64 `gorm:"column:transfer_token_value"`
	TransferBlockID    string  `gorm:"column:transfer_block_number_and_id"`
	Epoch              uint64  `gorm:"column:epoch"`
	NextBlockEpoch     uint64  `gorm:"column:next_epoch"`
	TokenCredit        uint64  `gorm:"column:token_credit"`
	TokenCreditStatus  int     `gorm:"column:token_credit_status"`
	RemainingCredits   uint64  `gorm:"column:remaining_credits"`
}

type MiningRecordPubSub struct {
	MiningID         string          `json:"mining_id"`
	MinedTokenID     string          `json:"miner_token_id"`
	MinerDID         string          `json:"miner_did"`
	TokenLevel       int             `json:"token_level"`
	TokenNumber      int             `json:"token_number"`
	RemainingCredits uint64          `json:"remaining_credits"`
	PledgeHistory    []PledgeHistory `json:"pledge_history"`
}

type GetTotalCredits struct {
	BasicResponse
	CreditDetails CredDetails
}

type CredDetails struct {
	Did          string
	TotalCredits uint64
}
