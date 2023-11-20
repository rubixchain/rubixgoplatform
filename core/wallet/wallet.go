package wallet

import (
	"fmt"
	"sync"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	TokenStorage                   string = "TokensTable"
	DataTokenStorage               string = "DataTokensTable"
	NFTTokenStorage                string = "NFTTokensTable"
	CreditStorage                  string = "CreditsTable"
	DIDStorage                     string = "DIDTable"
	DIDPeerStorage                 string = "DIDPeerTable"
	TransactionStorage             string = "TransactionHistory"
	TokensArrayStorage             string = "TokensTransferred"
	TokenProvider                  string = "TokenProviderTable"
	TokenChainStorage              string = "tokenchainstorage"
	NFTChainStorage                string = "nftchainstorage"
	DataChainStorage               string = "datachainstorage"
	SmartContractTokenChainStorage string = "smartcontractokenchainstorage"
	SmartContractStorage           string = "smartcontract"
	CallBackUrlStorage             string = "callbackurl"
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
	leveldb.DB
	l sync.Mutex
}

type Wallet struct {
	ipfs                           *ipfsnode.Shell
	s                              storage.Storage
	l                              sync.Mutex
	dtl                            sync.Mutex
	log                            logger.Logger
	wl                             sync.Mutex
	tcs                            *ChainDB
	dtcs                           *ChainDB
	ntcs                           *ChainDB
	smartContractTokenChainStorage *ChainDB
}

func InitWallet(s storage.Storage, dir string, log logger.Logger) (*Wallet, error) {
	var err error
	w := &Wallet{
		log: log.Named("wallet"),
		s:   s,
	}
	w.tcs = &ChainDB{}
	w.dtcs = &ChainDB{}
	w.ntcs = &ChainDB{}
	w.smartContractTokenChainStorage = &ChainDB{}
	op := &opt.Options{
		WriteBuffer: 64 * 1024 * 1024,
	}

	tdb, err := leveldb.OpenFile(dir+TokenChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	w.tcs.DB = *tdb
	ntdb, err := leveldb.OpenFile(dir+NFTChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure NFT chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure NFT chain block storage")
	}
	w.ntcs.DB = *ntdb
	dtdb, err := leveldb.OpenFile(dir+DataChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure data chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure data chain block storage")
	}
	w.dtcs.DB = *dtdb
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
	err = w.s.Init(DataTokenStorage, &DataToken{}, true)
	if err != nil {
		w.log.Error("Failed to initialize data token storage", "err", err)
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
	err = w.s.Init(TransactionStorage, &TransactionDetails{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Transaction storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokenProvider, &TokenProviderMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Token Provider Table", "err", err)
		return nil, err
	}
	err = w.s.Init(SmartContractStorage, &SmartContract{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Smart Contract storage", "err", err)
		return nil, err
	}

	smartcontracTokenchainstorageDB, err := leveldb.OpenFile(dir+SmartContractTokenChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	w.smartContractTokenChainStorage.DB = *smartcontracTokenchainstorageDB

	err = w.s.Init(CallBackUrlStorage, &CallBackUrl{}, true)
	if err != nil {
		w.log.Error("Failed to initialize Smart Contract Callback Url storage", "err", err)
		return nil, err
	}

	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
}
