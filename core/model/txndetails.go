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
