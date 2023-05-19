package model

import "github.com/rubixchain/rubixgoplatform/core/wallet"

type PendingTxnDetails struct {
	BasicResponse
	TxnDetails []wallet.TxnDetails
}

type PendingTxnIds struct {
	BasicResponse
	TxnIds []string
}
