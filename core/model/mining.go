package model

type MiningRequest struct {
	MinerDid     string `json:"miner"`
	TokenCredits int    `json:"credits"`
	Password     string `json:"password"`
}

type PledgeHistory struct {
	QuorumDID            string  `gorm:"column:quorum_did"`
	TransactionID        string  `gorm:"column:transaction_id"`
	TransactionType      int     `gorm:"column:transaction_type"`
	TransferTokenID      string  `gorm:"column:transfer_tokens_id"`
	TransferTokenType    int     `gorm:"column:transfer_tokens_type"`
	TransferTokenValue   float64 `gorm:"column:transfer_token_value"`
	TransferBlockID      string  `gorm:"column:transfer_block_number_and_id"`
	LatestTokenStateHash string  `gorm:"column:latest_tokenstate_hash"`
	Epoch                int     `gorm:"column:epoch"`
	NextBlockEpoch       int64   `gorm:"column:next_epoch"`
	TokenCredit          int     `gorm:"column:token_credit"`
	TokenCreditStatus    int     `gorm:"column:token_credit_status"`
}
