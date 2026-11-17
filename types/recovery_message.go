package types

import "encoding/json"

const RecoveryMessageVersion = 1

const (
	RecoveryMessageCommitment = "commitment"
	RecoveryMessageUnpledged  = "unpledged"
	RecoveryMessageChallenge  = "challenge"
	RecoveryMessageMismatch   = "mismatch"
	RecoveryMessageCompletion = "completion"
)

// RecoveryMessage carries recovery data in the child NFT's nft_data.
// Type tells the receiver how to read Body.
type RecoveryMessage struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Body      json.RawMessage `json:"body"`
	SenderDID string          `json:"sender_did"`
	Signature string          `json:"signature,omitempty"`
}
