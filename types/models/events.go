package models

type EventTransaction struct {
	Transaction *Transactions `json:"transaction"`
	Status      bool          `json:"status"`
	Message     string        `json:"message"`
	BlockHash   string        `json:"block_hash"`
	AssetType   int           `json:"asset_type"`
}
