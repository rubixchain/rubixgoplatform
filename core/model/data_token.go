package model

import "github.com/rubixchain/rubixgoplatform/core/wallet"

type DataTokenResponse struct {
	BasicResponse
	Tokens []wallet.DataToken `json:"tokens"`
}
