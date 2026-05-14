package model

// BasicResponse will be basic response model
type BasicResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

// MintedChild reports one server-generated child NFT ID back to the caller
// when a transaction includes child-mint entries.
type MintedChild struct {
	ParentNFTId string `json:"parentNFTId"`
	ChildNFTId  string `json:"childNFTId"`
}

// TransactionResult is the typed payload returned via BasicResponse.Result
// from InitiateTransaction on success. MintedNFTChildren is omitted from the
// JSON when no children were minted in the transaction.
type TransactionResult struct {
	TransactionID     string        `json:"transactionID"`
	MintedNFTChildren []MintedChild `json:"mintedNFTChildren"`
}

// TokenNumberResponse will be basic response model
type TokenNumberResponse struct {
	Status       bool   `json:"status"`
	Message      string `json:"message"`
	TokenNumbers []int  `json:"tokennumbers"`
}

// MigratedToken Check
type MigratedTokenStatus struct {
	Status         bool   `json:"status"`
	Message        string `json:"message"`
	MigratedStatus []int  `json:"migratedstatus"`
}
