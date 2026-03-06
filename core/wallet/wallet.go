package wallet

import (
	"context"
	"fmt"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	TokenStorage                   string = "TokensTable"
	NFTTokenStorage                string = "NFTTokensTable"
	CreditStorage                  string = "CreditsTable"
	DIDStorage                     string = "DIDTable"
	DIDPeerStorage                 string = "DIDPeerTable"
	TransactionStorage             string = "TransactionHistory"
	TokensArrayStorage             string = "TokensTransferred"
	TokenProvider                  string = "TokenProviderTable"
	TokenChainStorage              string = "tokenchainstorage"
	NFTChainStorage                string = "nftchainstorage"
	SmartContractTokenChainStorage string = "smartcontractokenchainstorage"
	SmartContractStorage           string = "smartcontract"
	CallBackUrlStorage             string = "callbackurl"
	TokenStateHash                 string = "TokenStateHashTable"
	UnpledgeQueueTable             string = "unpledgequeue"
	UnpledgeSequence               string = "UnpledgeSequence"
	FTTokenStorage                 string = "FTTokenTable"
	FTChainStorage                 string = "FTchainstorage"
	FTStorage                      string = "FTTable"
	FTTransactionTokenStorage      string = "FTTransactionTokens"
	FailedFTDownloadStorage        string = "FailedFTDownloads"
	FullNodeStorage                string = "Fullnodestorage"
	FullNodeRBTTable               string = "FullnodeRBTtable"
	FullNodeFTTable                string = "FullnodeFTtable"
	FullNodeNFTTable               string = "FullnodeNFTtable"
	FullNodeSmartContractTable     string = "FullnodeSCtable"
	FullNodeTxnHistoryTable        string = "FullnodeTxnHistoryTable"
	FailedTxnsTable                string = "FailedTxns"
	FullNodeFailedToSyncTokens     string = "FullnodeFailedTokensTable"
	FullNodeRBTContentTable        string = "rbt_content_table"
	FullNodeFTContentTable         string = "ft_content_table"
	FullNodeNFTContentTable        string = "nft_content_table"
	FullNodeSCContentTable         string = "sc_content_table"
	FullnodeDoubleSpentTokensTable string = "DoubleSpentTokensTable"
	LocalTestTokenInfo             string = "LocalTestTokenInfo"
)

type Wallet struct {
	ipfs                           *ipfsnode.Shell
	ipfsOps                        types.IPFSOperations
	log                            logger.Logger
	asyncProviderMgr               *AsyncProviderDetailsManager
	db                             *storage.RubixDB
}


// GetIpfsOps returns the IPFS operations interface
func (w *Wallet) GetIpfsOps() types.IPFSOperations {
	return w.ipfsOps
}

func NewWallet(ctx context.Context, db *storage.RubixDB, log logger.Logger) (*Wallet, error) {
	w := &Wallet{
		log: log.Named("wallet"),
		db:  db,
	}

	err := w.db.InitSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise table schema: %v", err)
	}

	// Initialize async provider details manager with 2 workers
	w.asyncProviderMgr = NewAsyncProviderDetailsManager(w, 2)

	// Initialize async provider details manager with 2 workers
	w.asyncProviderMgr = NewAsyncProviderDetailsManager(w, 2)

	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
	// Default to direct IPFS operations if no health-managed operations are set
	if w.ipfsOps == nil {
		w.ipfsOps = types.NewDirectIPFSOperations(ipfs)
	}
}

// SetIPFSOperations sets the IPFS operations interface (for health-managed operations)
func (w *Wallet) SetIPFSOperations(ops types.IPFSOperations) {
	w.ipfsOps = ops
}

