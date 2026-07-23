package types

// RecoverWalletAdvancedRequest is the operator-facing body for an advanced
// recovery run. DID is required unless AllDIDs is set, in which case every local
// DID on the node is recovered. Mode is full, delta, or dryrun; empty is full.
type RecoverWalletAdvancedRequest struct {
	DID        string   `json:"did"`
	Mode       string   `json:"mode,omitempty"`
	TokenTypes []string `json:"token_types,omitempty"`
	TokenIDs   []string `json:"token_ids,omitempty"`
	SelfTest   bool     `json:"self_test,omitempty"`
	AllDIDs    bool     `json:"all_dids,omitempty"`
}
