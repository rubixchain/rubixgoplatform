package types

import "github.com/rubixchain/rubixgoplatform/types/models"

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
