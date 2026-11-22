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

// RecoveryDeliveryCheck is the user's verdict on one committer, a quorum or a
// fullnode: the commitment it published, and the one the user rebuilt from
// the chains it delivered. Recomputed is nil when nothing was delivered or
// the chains could not be hashed.
type RecoveryDeliveryCheck struct {
	CommitterDID string              `json:"committer_did"`
	Published    RecoveryCommitment  `json:"published"`
	Recomputed   *RecoveryCommitment `json:"recomputed,omitempty"`
	Match        bool                `json:"match"`
	Mismatches   []string            `json:"mismatches,omitempty"`
}

// RecoveryVerification is the user's check of every published quorum and
// unpledged commitment after the sync window closes [R12].
type RecoveryVerification struct {
	RequestID string                  `json:"request_id"`
	UserDID   string                  `json:"user_did"`
	Checks    []RecoveryDeliveryCheck `json:"checks"`
}

// RecoveryMismatchReport is the body of a mismatch message [R13]: the
// failing checks of the user's verification.
type RecoveryMismatchReport struct {
	RequestID  string                  `json:"request_id"`
	Mismatches []RecoveryDeliveryCheck `json:"mismatches"`
}

// RecoveryCompletionNotice is the body of a completion message [R13].
type RecoveryCompletionNotice struct {
	RequestID string `json:"request_id"`
}

// RecoveryChainsNotice is what a committer, a quorum or a fullnode, sends the
// user when the sync window opens, the way send_tokens tells a receiver about
// a transfer. The user then pulls the chains from the committer: from a
// quorum with sync_transaction_chain, from a fullnode with
// recovery_fullnode_chains. CommitterType is "quorum" or "fullnode".
// Signature is the committer DID's signature over the notice with an empty
// Signature field.
type RecoveryChainsNotice struct {
	RequestID       string                    `json:"request_id"`
	CommitterDID    string                    `json:"committer_did"`
	CommitterType   string                    `json:"committer_type"`
	CommitterPeerID string                    `json:"committer_peer_id"`
	Tokens          []RecoveryTokenCommitment `json:"tokens"`
	Signature       string                    `json:"signature"`
}
