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

// RecoveryMessage carries recovery data in the ephemeral NFT's nft_data.
// Type tells the receiver how to read Body.
type RecoveryMessage struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	Body      json.RawMessage `json:"body"`
	SenderDID string          `json:"sender_did"`
}
