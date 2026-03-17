package types

// IPFSProviderContext carries metadata for IPFS provider tracking.
// Pass nil to skip provider logging (e.g. for OnlyHash operations or infrastructure calls).
type IPFSProviderContext struct {
	DID           string
	Role          int
	TransactionID string
	ResourceType  string
	ResourceID    string
	Initiator     string  // who initiated the operation (sender DID)
	Owner         string  // token owner DID
	TokenValue    float64 // token monetary value
}
