package wallet

import (
	"fmt"
	"sync"

	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	TokenStorage       string = "TokensTable"
	PartTokenStorage   string = "PartTokensTable"
	DataTokenStorage   string = "DataTokensTable"
	NFTTokenStorage    string = "NFTTokensTable"
	CreditStorage      string = "CreditsTable"
	DIDStorage         string = "DIDTable"
	DIDPeerStorage     string = "DIDPeerTable"
	TransactionStorage string = "TransactionHistory"
	TokensArrayStorage string = "TokensTransferred"
	QuorumListStorage  string = "QuorumList"
	TokenProvider      string = "TokenProviderTable"
	IPFSFunction       string = "IpfsFunctionTable"
	UserRole           string = "RoleTable"
	TokenChainStorage  string = "tokenchainstorage"
	NFTChainStorage    string = "nftchainstorage"
	DataChainStorage   string = "datachainstorage"
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
	ipfs *ipfsnode.Shell
	s    storage.Storage
	l    sync.Mutex
	dtl  sync.Mutex
	log  logger.Logger
	wl   sync.Mutex
	tcs  *ChainDB
	dtcs *ChainDB
}

func InitWallet(s storage.Storage, dir string, log logger.Logger) (*Wallet, error) {
	var err error
	w := &Wallet{
		log: log.Named("wallet"),
		s:   s,
	}
	w.tcs = &ChainDB{}
	w.dtcs = &ChainDB{}
	tdb, err := leveldb.OpenFile(dir+TokenChainStorage, nil)
	if err != nil {
		w.log.Error("failed to configure token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	w.tcs.DB = *tdb
	dtdb, err := leveldb.OpenFile(dir+DataChainStorage, nil)
	if err != nil {
		w.log.Error("failed to configure data chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure data chain block storage")
	}
	w.dtcs.DB = *dtdb
	err = w.s.Init(DIDStorage, &DIDType{})
	if err != nil {
		w.log.Error("Failed to initialize DID storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokenStorage, &Token{})
	if err != nil {
		w.log.Error("Failed to initialize whole token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(PartTokenStorage, &PartToken{})
	if err != nil {
		w.log.Error("Failed to initialize part token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(DataTokenStorage, &DataToken{})
	if err != nil {
		w.log.Error("Failed to initialize data token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(CreditStorage, &Credit{})
	if err != nil {
		w.log.Error("Failed to initialize credit storage", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage, &DIDPeerMap{})
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TransactionStorage, &TransactionHistory{})
	if err != nil {
		w.log.Error("Failed to initialize Transaction storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokensArrayStorage, &TokensTransferred{})
	if err != nil {
		w.log.Error("Failed to initialize Tokens Array storage", "err", err)
		return nil, err
	}
	err = w.s.Init(QuorumListStorage, &QuorumList{})
	if err != nil {
		w.log.Error("Failed to initialize Quorum List storage", "err", err)
		return nil, err
	}
	err = w.s.Init(TokenProvider, &TokenProviderMap{})
	if err != nil {
		w.log.Error("Failed to initialize Token Provider Table", "err", err)
		return nil, err
	}
	err = w.s.Init(IPFSFunction, &Function{})
	if err != nil {
		w.log.Error("Failed to initialize IPFS Functions Table", "err", err)
		return nil, err
	}
	err = w.s.Init(UserRole, &Role{})
	if err != nil {
		w.log.Error("Failed to initialize User Role Table", "err", err)
		return nil, err
	}
	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
}
