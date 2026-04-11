package models

type EventTransaction struct {
	Transaction *Transactions `json:"transaction"`
	Status      bool          `json:"status"`
	Message     string        `json:"message"`
	TransactionID string      `json:"transaction_id"`
	AssetType   int           `json:"asset_type"`
}

type EventSmartContractPublishInfo struct {
	SmartContractID    string `json:"smartcontract_id"`
	TransactionID      string `json:"transaction_id"`
	Initiator          string `json:"initiator"`
	InitiatorSignature string `json:"initiator_signature"`
	Epoch              int    `json:"epoch"`
	SmartContractData  string `json:"smartcontract_data"`
}

type EventNFTPublishInfo struct {
	NFTid              string `json:"nft_id"`
	TransactionID      string `json:"transaction_id"`
	Initiator          string `json:"initiator"`
	InitiatorSignature string `json:"initiator_signature"`
	Epoch              int    `json:"epoch"`
	NFTData            string `json:"nft_data"`
}
