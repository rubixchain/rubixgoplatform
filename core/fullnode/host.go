package fullnode

import (
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Host is what the fullnode pipeline needs from the node it runs inside.
//
// This package cannot define methods on core.Core, and cannot import core
// without closing an import cycle. Host is the seam: core implements it and
// passes an implementation to newTxnProcessor, so the dependency runs one way
// only.
//
// It carries node state (the wallet, the listener, the network flags) and
// operations that need the rest of the node to perform (resolving a DID,
// reaching a peer). Nothing on it is pipeline logic.
type Host interface {
	Log() logger.Logger
	Wallet() *wallet.Wallet
	Listener() *ipfsport.Listener
	IsFullNode() bool

	// NetworkFlags returns testnet, mainnet and localnet together; validation
	// takes all three and they are only ever read as a set.
	NetworkFlags() (testnet, mainnet, localnet bool)

	InitialiseDID(did string) (types.DIDCrypto, error)
	SyncTransactionChainsFromPeer(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string, transferNFTOwnership bool, isFullnode bool) error
	SyncTokensFromFullnode(tokenIDs []string) (map[string]string, error)
	FetchGenesisTransactionFromPeer(peerDID, tokenID string) (*models.Transactions, error)
	GetTransactionInfoByID(txID string) (*models.TransactionInfo, error)
	GetParentBurnTxID(parentID string) (string, bool, error)

	// CheckTokenStateHashPinned stays on the node because the quorum path calls
	// it too (core/quorum_initiator.go).
	CheckTokenStateHashPinned(tokenID, previousTransactionID string) error

	// CPUUsage reports utilisation since the previous sample and returns the new
	// sample to pass back. The worker pool scales on it.
	CPUUsage(lastStats map[string]uint64) (float64, map[string]uint64)
}
