package model

import (
	"time"
)

type TransactionDetails struct {
	TransactionID   string           `gorm:"column:transaction_id;primaryKey"`
	TransactionType string           `gorm:"column:transaction_type"`
	BlockID         string           `gorm:"column:block_id"`
	Mode            int              `gorm:"column:mode"`
	SenderDID       string           `gorm:"column:sender_did"`
	ReceiverDID     string           `gorm:"column:receiver_did"`
	Amount          float64          `gorm:"column:amount"`
	TotalTime       float64          `gorm:"column:total_time"`
	Comment         string           `gorm:"column:comment"`
	DateTime        time.Time        `gorm:"column:date_time"`
	Status          bool             `gorm:"column:status"`
	DeployerDID     string           `gorm:"column:deployer_did"`
	Epoch           int64            `gorm:"column:epoch"`
	Tokens          []FTTokenSummary `gorm:"-"`
}

type TransactionCount struct {
	DID         string
	TxnSend     int
	TxnReceived int
}

type FTTokenSummary struct {
	CreatorDID string
	FTName     string
	Count      int
}

type TxnDetails struct {
	BasicResponse
	TxnDetails []TransactionDetails
}

type TxnCountForDID struct {
	BasicResponse
	TxnCount []TransactionCount
}

type PubSubTxnInfo struct {
	BlockHash                string  `gorm:"column:block_hash;primaryKey"`
	TransactionID            string  `gorm:"column:transaction_id"`
	TxnType                  string  `gorm:"column:transaction_type"`
	AssetType                int     `gorm:"column:asset_type"`
	FTName                   string  `gorm:"column:ft_name"`
	CreatorDID               string  `gorm:"column:creator_did"`
	PublisherDID             string  `gorm:"column:publisher_did"`
	ReceiverDID              string  `gorm:"column:receiver_did"`
	TxnBlock                 []byte  `gorm:"column:block"`
	LatestBlockHeight        uint64  `gorm:"column:block_height"`
	TransactionValue         float64 `gorm:"column:transaction_value"`
	TokenValue               float64 `gorm:"column:token_value"`
	FullNodeAsProviderPeerID string  `gorm:"column:fullnode_publisher_peer_ID"` //In case of full nodes syncing among themself, fullnode act as a block provider to other fullnode
}

type FailedTransaction struct {
	BlockHash    string    `gorm:"column:txn_id"`
	PublisherDID string    `gorm:"column:publisher_did"`
	Error        string    `gorm:"column:error"`
	FailedAt     time.Time `gorm:"column:failed_at"`
	RetryCount   int       `gorm:"column:retry_count"`
}
