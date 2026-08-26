package models

type EventTransaction struct {
	Transaction   *Transactions `json:"transaction"`
	Status        bool          `json:"status"`
	Message       string        `json:"message"`
	TransactionID string        `json:"transaction_id"`
	AssetType     int           `json:"asset_type"`
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
	NFTid                string `json:"nft_id"`
	TransactionID        string `json:"transaction_id"`
	Initiator            string `json:"initiator"`
	InitiatorSignature   string `json:"initiator_signature"`
	Epoch                int    `json:"epoch"`
	NFTData              string `json:"nft_data"`
	NFTOwnershipTransfer bool   `json:"nft_ownership_transfer,omitempty"`
	// IsBurn marks this event as an NFT burn. Subscribers use it to
	// distinguish a terminal destruction from an ordinary state change: after
	// a burn the NFT can never be transacted again, so a subscriber should
	// stop treating it as live rather than merely re-syncing its chain.
	IsBurn bool `json:"is_burn,omitempty"`
}

type EventUnpledgeInfo struct {
	UnpledgeInfo          []MsgUnpledgeTokenInfo `json:"unpledgeInfo"`
	PledgeTransactionID   string                 `json:"pledgeTransactionID"`
	UnpledgeTransactionID string                 `json:"unpledgeTransactionID"`
}

type MsgUnpledgeTokenInfo struct {
	TokenID               string `json:"tokenId"`
}
