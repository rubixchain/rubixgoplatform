package model

import "github.com/rubixchain/rubixgoplatform/core/wallet"

type TxnDetails struct {
	BasicResponse
	TxnDetails []wallet.TransactionDetails
}
