package types

import "github.com/rubixchain/rubixgoplatform/types/models"

type TransactionWithRole struct {
	Tx   models.Transactions
	Role int16
}
