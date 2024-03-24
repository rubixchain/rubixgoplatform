package model

import "github.com/rubixchain/rubixgoplatform/core/wallet"

type TxnDetails struct {
	BasicResponse
	TxnDetails []wallet.TransactionDetails
}

type TxnCountForDID struct {
	BasicResponse
	TxnCount []wallet.TransactionCount
}
