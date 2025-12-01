package wallet

import (
	"fmt"
	"sync"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
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
	FullNodeBlockHashTable         string = "FullnodeBlockHashTable"
	FullNodeTxnHistoryTable        string = "FullnodeTxnHistoryTable"
	FailedTxnsTable                string = "FailedTxns"
	FullNodeFailedToSyncTokens     string = "FullnodeFailedTokensTable"
	FullNodeRBTContentTable        string = "rbt_content_table"
	FullNodeFTContentTable         string = "ft_content_table"
	FullNodeNFTContentTable        string = "nft_content_table"
	FullNodeSCContentTable         string = "sc_content_table"
	FullnodeDoubleSpentTokensTable string = "DoubleSpentTokensTable"
)

type WalletConfig struct {
	StorageType   int    `json:"stroage_type"`
	DBName        string `json:"db_name"`
	DBAddress     string `json:"db_address"`
	DBPort        string `json:"db_port"`
	DBType        string `json:"db_type"`
	DBUserName    string `json:"db_user_name"`
	DBPassword    string `json:"db_password"`
	TokenChainDir string `json:"token_chain_dir"`
}

type ChainDB struct {
	*leveldb.DB
	l sync.Mutex
}

type Wallet struct {
	ipfs                           *ipfsnode.Shell
	ipfsOps                        IPFSOperations
	s                              storage.Storage
	fullNodeSQLDB                  storage.Storage
	fullNodePSQLTokensDB           storage.Storage
	l                              sync.Mutex
	dtl                            sync.Mutex
	log                            logger.Logger
	wl                             sync.Mutex
	tcs                            *ChainDB
	ntcs                           *ChainDB
	smartContractTokenChainStorage *ChainDB
	FTChainStorage                 *ChainDB
	asyncProviderMgr               *AsyncProviderDetailsManager
	fullNodeStorage                *ChainDB
	IsFullNode                     bool
}

// GetStorage returns the storage interface
func (w *Wallet) GetStorage() storage.Storage {
	return w.s
}

// GetIpfsOps returns the IPFS operations interface
func (w *Wallet) GetIpfsOps() IPFSOperations {
	return w.ipfsOps
}

func InitWallet(s storage.Storage, fullNodeSQLDB storage.Storage, fullNodePSQLTokensDB storage.Storage, dir string, log logger.Logger, fullNode bool) (*Wallet, error) {
	var err error
	w := &Wallet{
		log:                  log.Named("wallet"),
		s:                    s,
		fullNodeSQLDB:        fullNodeSQLDB,
		fullNodePSQLTokensDB: fullNodePSQLTokensDB,
		IsFullNode:           fullNode,
	}
	w.tcs = &ChainDB{}
	w.ntcs = &ChainDB{}
	w.smartContractTokenChainStorage = &ChainDB{}
	w.FTChainStorage = &ChainDB{}
	w.fullNodeStorage = &ChainDB{}
	op := &opt.Options{
		WriteBuffer: 64 * 1024 * 1024,
	}

	tdb, err := leveldb.OpenFile(dir+TokenChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	w.tcs.DB = tdb
	ntdb, err := leveldb.OpenFile(dir+NFTChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure NFT chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure NFT chain block storage")
	}
	w.ntcs.DB = ntdb

	err = w.s.Init(DIDStorage, &DIDType{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokenStorage, &Token{}, true)
	if err != nil {
		w.log.Error("Failed to initialize whole token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(NFTTokenStorage, &NFT{}, true)
	if err != nil {
		w.log.Error("Failed to initialize data token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(CreditStorage, &Credit{}, true)
	if err != nil {
		w.log.Error("Failed to initialize credit storage", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TransactionStorage, &model.TransactionDetails{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Transaction storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokenProvider, &model.TokenProviderMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Token Provider Table", "err", err)
		return nil, err
	}
	err = w.s.Init(SmartContractStorage, &SmartContract{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Smart Contract storage", "err", err)
		return nil, err
	}
	err = w.s.Init(UnpledgeSequence, &UnpledgeSequenceInfo{}, true)
	if err != nil {
		w.log.Error("failed to init UnpledgeSequence table", "err", err)
		return nil, err
	}
	err = w.s.Init(FTTokenStorage, FTToken{}, true)
	if err != nil {
		w.log.Error("Failed to initialize FT Token storage", "err", err)
	}
	err = w.s.Init(FTStorage, &FT{}, true)
	if err != nil {
		w.log.Error("Failed to initialize FT storage", "err", err)
	}
	err = w.s.Init(FTTransactionTokenStorage, &model.FTTransactionToken{}, true)
	if err != nil {
		w.log.Error("Failed to initialize FT transaction token storage", "err", err)
	}
	err = w.s.Init(FTTransactionHistoryStorage, &model.FTTransactionHistory{}, true)
	if err != nil {
		w.log.Error("Failed to initialize FT transaction history storage", "err", err)
	}
	// Initialize token recovery tracking table
	err = w.s.Init("TokenRecovery", &model.TokenRecovery{}, true)
	if err != nil {
		w.log.Error("Failed to initialize token recovery storage", "err", err)
	}

	smartcontracTokenchainstorageDB, err := leveldb.OpenFile(dir+SmartContractTokenChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure smart contract token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure smart contract token chain block storage")
	}
	w.smartContractTokenChainStorage.DB = smartcontracTokenchainstorageDB

	FTtokenStorageDB, err := leveldb.OpenFile(dir+FTChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure FT token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure FT token chain block storage")
	}
	w.FTChainStorage.DB = FTtokenStorageDB
	err = w.s.Init(CallBackUrlStorage, &CallBackUrl{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Smart Contract Callback Url storage", "err", err)
		return nil, err
	}

	err = w.s.Init(TokenStateHash, &TokenStateDetails{}, true)
	if err != nil {
		w.log.Error("Failed to initialize TokenStateHash", "err", err)
		return nil, err
	}

	// Initialize async provider details manager with 2 workers
	w.asyncProviderMgr = NewAsyncProviderDetailsManager(w, 2)

	// Initialize async provider details manager with 2 workers
	w.asyncProviderMgr = NewAsyncProviderDetailsManager(w, 2)

	// DB for fullnodes to store all token-chains
	if w.IsFullNode {
		fullNodeDB, err := leveldb.OpenFile(dir+FullNodeStorage, op)
		if err != nil {
			w.log.Error("failed to configure token chain block storage for full node", "err", err)
			return nil, fmt.Errorf("failed to configure token chain block storage for full node")
		}
		w.fullNodeStorage.DB = fullNodeDB

		err = w.fullNodeSQLDB.Init(FullNodeRBTTable, &SyncedRBT{}, true)
		if err != nil {
			w.log.Error("Failed to initialize RBT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FullNodeFTTable, &SyncedFT{}, true)
		if err != nil {
			w.log.Error("Failed to initialize FT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FullNodeNFTTable, &SyncedNFT{}, true)
		if err != nil {
			w.log.Error("Failed to initialize NFT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FullNodeSmartContractTable, &SyncedSmartContract{}, true)
		if err != nil {
			w.log.Error("Failed to initialize fullnode smart contract token storage", "err", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FullNodeBlockHashTable, &ReceivedBlockHash{}, true)
		if err != nil {
			w.log.Error("Failed to initialize RBT token storage", "err", err)
			return nil, err
		}
		// Create triggers after tables exist
		if err := w.setupTriggers(); err != nil {
			w.log.Error("Failed to create triggers", "err", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FailedTxnsTable, &model.FailedTransaction{}, true)
		if err != nil {
			w.log.Error("Failed to initialize fullnode failed transaction storage", "err", err)
			return nil, err
		}
		err = w.fullNodeSQLDB.Init(FullNodeFailedToSyncTokens, &model.FailedToSyncTokenDetailsInfo{}, true)
		if err != nil {
			w.log.Error("failed to initialize FullNodeFailedToSyncTokens storage", "error", err)
			return nil, err
		}

		err = w.fullNodeSQLDB.Init(FullnodeDoubleSpentTokensTable, &model.DoubleSpentTokenInfo{}, true)
		if err != nil {
			w.log.Error("failed to initialize FullnodeDoubleSpentTokensTable storage", "error", err)
			return nil, err
		}
		err = w.fullNodeSQLDB.Init(FullNodeTxnHistoryTable, &model.FullNodeTxnHistoryInfo{}, true)
		if err != nil {
			w.log.Error("failed to initialize FullNodeTxnHistoryTable storage", "error", err)
		}

		err = w.fullNodePSQLTokensDB.Init(FullNodeRBTContentTable, &RBTContent{}, true)
		if err != nil {
			w.log.Error("Failed to initialize postgres RBT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodePSQLTokensDB.Init(FullNodeFTContentTable, &FTContent{}, true)
		if err != nil {
			w.log.Error("Failed to initialize postgres FT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodePSQLTokensDB.Init(FullNodeNFTContentTable, &NFTContent{}, true)
		if err != nil {
			w.log.Error("Failed to initialize NFT token storage", "err", err)
			return nil, err
		}

		err = w.fullNodePSQLTokensDB.Init(FullNodeSCContentTable, &SmartContractContent{}, true)
		if err != nil {
			w.log.Error("Failed to initialize fullnode smart contract token storage", "err", err)
			return nil, err
		}
	}

	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
	// Default to direct IPFS operations if no health-managed operations are set
	if w.ipfsOps == nil {
		w.ipfsOps = NewDirectIPFSOperations(ipfs)
	}
}

// SetIPFSOperations sets the IPFS operations interface (for health-managed operations)
func (w *Wallet) SetIPFSOperations(ops IPFSOperations) {
	w.ipfsOps = ops
}

// Re-export StorageType for convenience
// StorageType is used for batch writes (Key, Value)
type StorageType = storage.StorageType

// S returns the storage interface (for batch writes)
func (w *Wallet) S() storage.Storage {
	return w.s
}
