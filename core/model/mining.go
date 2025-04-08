package model

type MiningRequest struct {
	MinerDid     string `json:"miner"`
	TokenCredits uint64 `json:"credits"`
	Password     string `json:"password"`
	Type         int    `json:"type"`
}

type PledgeHistory struct {
	QuorumDID          string  `gorm:"column:quorum_did"`
	TransactionID      string  `gorm:"column:transaction_id"`
	TransactionType    int     `gorm:"column:transaction_type"`
	TransferTokenID    string  `gorm:"column:transfer_tokens_id"`
	TransferTokenType  int     `gorm:"column:transfer_tokens_type"`
	TransferTokenValue float64 `gorm:"column:transfer_token_value"`
	TransferBlockID    string  `gorm:"column:transfer_block_number_and_id"`
	Epoch              int     `gorm:"column:epoch"`
	NextBlockEpoch     int64   `gorm:"column:next_epoch"`
	TokenCredit        int     `gorm:"column:token_credit"`
	TokenCreditStatus  int     `gorm:"column:token_credit_status"`
}

type MiningRecordPubSub struct {
	MiningID                 string                `json:"mining_id"`
	MinedTokenID             string                `json:"miner_token_id"`
	MinerDID                 string                `json:"miner_did"`
	TokenLevelAndTokenNumber int                   `json:"token_level_and_token_number"`
	QuorumList               []QuorumDIDSignatures `json:"quorumlist"`
	PledgeHistory            []PledgeHistory       `json:"pledge_history"`
}

type QuorumDIDSignatures struct {
	DID       string `json:"did"`
	Signature string `json:"signature"`
}
