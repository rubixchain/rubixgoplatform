package fullnode

import (
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// Host is what the fullnode pipeline needs from the node it runs inside.
// core.Core implements it and passes itself to NewTxnProcessor; this package
// cannot import core without an import cycle.
type Host interface {
	Log() logger.Logger
	Wallet() *wallet.Wallet
	Listener() *ipfsport.Listener
	IsFullNode() bool

	// NetworkFlags returns testnet, mainnet and localnet, which validation takes together.
	NetworkFlags() (testnet, mainnet, localnet bool)

	InitialiseDID(did string) (types.DIDCrypto, error)
	SyncTransactionChainsFromPeer(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string, transferNFTOwnership bool, isFullnode bool) error
	SyncTokensFromFullnode(tokenIDs []string) (map[string]string, error)
	FetchGenesisTransactionFromPeer(peerDID, tokenID string) (*models.Transactions, error)
	SyncBurntTokenChainFromPeer(peerDID, tokenID string) error
	VerifyGenesisSignature(signerDID string, info *models.TransactionInfo, signature string) error
	GetTransactionInfoByID(txID string) (*models.TransactionInfo, error)
	GetParentBurnTxID(parentID string) (string, bool, error)

	// CheckTokenStateHashPinned stays on the node because quorum validation uses it too.
	CheckTokenStateHashPinned(tokenID, previousTransactionID string) error

	// CPUUsage reports utilisation since lastStats and returns the new sample.
	CPUUsage(lastStats map[string]uint64) (float64, map[string]uint64)

	// MemoryUsagePercent reports memory utilisation, 0-100.
	MemoryUsagePercent() float64
}
