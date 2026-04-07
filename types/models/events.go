package models

type EventTransaction struct {
	Transaction *Transactions `json:"transaction"`
	Status      bool          `json:"status"`
	Message     string        `json:"message"`
	TransactionID string      `json:"transaction_id"`
	AssetType   int           `json:"asset_type"`
}
