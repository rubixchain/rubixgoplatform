package types

import "encoding/json"

// RecoveryChainEntry is one tokenchain row joined with its transaction's info
// JSON. Info is nil when the tokenchain row points at a transaction the node
// does not have.
type RecoveryChainEntry struct {
	TransactionID         string
	PreviousTransactionID string // "" at position 0
	Position              int64
	Role                  int16
	Info                  json.RawMessage
}

// RecoveryCommitment is the token-chain root a quorum or fullnode publishes
// for one recovery request. CommitterType (constants.RecoveryCommitterType_*)
// tells the receiver which token set it covers.
type RecoveryCommitment struct {
	RequestID     string `json:"request_id"`
	UserDID       string `json:"user_did"`
	CommitterDID  string `json:"committer_did"`
	CommitterType string `json:"committer_type"`
	TokenCount    int    `json:"token_count"`
	Root          string `json:"root"` // hex SHA3-256
}

// RecoveryTokenCommitment is one token's layer-1 result.
type RecoveryTokenCommitment struct {
	TokenID   string `json:"token_id"`
	ChainRoot string `json:"chain_root"` // hex SHA3-256 chain hash
}

// RecoveryCommitmentTree is a RecoveryCommitment plus everything needed to
// prove any token under it. Tokens is sorted by TokenID, which is the layer-2
// leaf order.
type RecoveryCommitmentTree struct {
	Commitment RecoveryCommitment        `json:"commitment"`
	Tokens     []RecoveryTokenCommitment `json:"tokens"`
}

// RecoveryExcludedToken records a candidate token that was left out of a
// commitment and why. It is diagnostic only and is not part of the committed
// root.
type RecoveryExcludedToken struct {
	TokenID string `json:"token_id"`
	Reason  string `json:"reason"`
}

// RecoveryProof is a Merkle audit path for the leaf at Index in a tree of Size
// leaves.
type RecoveryProof struct {
	Index int      `json:"index"`
	Size  int      `json:"size"`
	Path  []string `json:"path"` // hex hashes, leaf sibling first
}

// RecoveryCommitmentCheck is the fullnode's verdict on one quorum's
// commitment. Expected is nil only if the fullnode could not reproduce it.
type RecoveryCommitmentCheck struct {
	QuorumDID  string                  `json:"quorum_did"`
	Submitted  RecoveryCommitment      `json:"submitted"`
	Expected   *RecoveryCommitmentTree `json:"expected,omitempty"`
	Match      bool                    `json:"match"`
	Mismatches []string                `json:"mismatches,omitempty"`
}

// RecoveryCommitmentReport is the result of checking every submitted
// commitment for one recovery request. MissingQuorums should have committed
// at least one token but submitted nothing. Excluded tokens are in no
// quorum's expected set.
type RecoveryCommitmentReport struct {
	RequestID      string                    `json:"request_id"`
	UserDID        string                    `json:"user_did"`
	Checks         []RecoveryCommitmentCheck `json:"checks"`
	MissingQuorums []string                  `json:"missing_quorums,omitempty"`
	Excluded       []RecoveryExcludedToken   `json:"excluded,omitempty"`
}
