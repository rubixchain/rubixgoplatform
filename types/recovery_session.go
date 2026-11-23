package types

import (
	"encoding/json"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// RecoverySessionStatus is this node's view of one recovery, as the session
// status API returns it. Deadlines are the Unix times at which nodes close
// each window, grace period included.
type RecoverySessionStatus struct {
	RequestID      string                    `json:"request_txn_id"`
	EphemeralNFTID string                    `json:"ephemeral_nft_id"`
	UserDID        string                    `json:"user_did"`
	Role           string                    `json:"role"`
	State          string                    `json:"state"`
	RequestEpoch   int64                     `json:"request_epoch"`
	CommitClosesAt int64                     `json:"commit_closes_at"`
	SyncClosesAt   int64                     `json:"sync_closes_at"`
	Messages       []RecoveryMessageEntry    `json:"messages"`
	Report         *RecoveryCommitmentReport `json:"report,omitempty"`       // fullnode only
	Deliveries     []RecoveryDeliverySummary `json:"deliveries,omitempty"`   // user only
	Verification   *RecoveryVerification     `json:"verification,omitempty"` // user only, after the sync window
}

// RecoveryMessageEntry is one message on the ephemeral NFT that passed the
// sender rules.
type RecoveryMessageEntry struct {
	TxnID     string          `json:"txn_id"`
	Type      string          `json:"type"`
	SenderDID string          `json:"sender_did"`
	Body      json.RawMessage `json:"body" swaggertype:"object"`
}

// RecoveryDeliverySummary is one committer's chain delivery, from a quorum or
// a fullnode, stored on the user's node.
type RecoveryDeliverySummary struct {
	CommitterDID string `json:"committer_did"`
	TokenCount   int    `json:"token_count"`
}

// RecoverySessionResponse is the session status API response.
type RecoverySessionResponse struct {
	Status  bool                   `json:"status"`
	Message string                 `json:"message"`
	Result  *RecoverySessionStatus `json:"result"`
}

// RecoverySession is this node's view of one recovery request.
type RecoverySession struct {
	RequestID string
	// EphemeralNFTID is the topic of the request's ephemeral NFT. It is a
	// hash, so events on that topic find their session through it.
	EphemeralNFTID string
	UserDID        string
	Role           string // constants.RecoveryRole_*
	State          int    // constants.RecoveryState_*
	// RequestEpoch is the request transaction's epoch. Deadlines count from it.
	RequestEpoch int64
}

// RecoveryTx is one transaction from a recovery NFT's local chain.
type RecoveryTx struct {
	ID   string
	Role int16
	Info models.TransactionInfo
	Data string // the recovery NFT's data in this transaction
}
