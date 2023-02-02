package wallet

import (
	"fmt"
	"sync"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	MainTokenStorage     string = "TokensTable"
	MainPartTokenStorage string = "PartTokensTable"
	MainCreditStorage    string = "CreditsTable"
	TestTokenStorage     string = "TestTokensTable"
	TestPartTokenStorage string = "TestPartTokensTable"
	TestCreditStorage    string = "TestCreditsTable"
	DIDStorage           string = "DIDTable"
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

type Wallet struct {
	s                storage.Storage
	l                sync.Mutex
	testNet          bool
	tokenStorage     string
	partTokenStorage string
	creditStorage    string
	log              logger.Logger
	wl               sync.Mutex
	tcs              *leveldb.DB
}

func InitWallet(cfg *WalletConfig, log logger.Logger, testNet bool) (*Wallet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("invalid wallet configuration")
	}
	var err error
	w := &Wallet{
		log:     log,
		testNet: testNet,
	}
	if testNet {
		w.tokenStorage = TestTokenStorage
		w.partTokenStorage = TestPartTokenStorage
		w.creditStorage = TestCreditStorage
	} else {
		w.tokenStorage = MainTokenStorage
		w.partTokenStorage = MainPartTokenStorage
		w.creditStorage = MainCreditStorage
	}
	w.tcs, err = leveldb.OpenFile(cfg.TokenChainDir+"tokenchainstorage", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	switch cfg.StorageType {
	case storage.StorageDBType:
		scfg := &config.Config{
			DBName:     cfg.DBAddress,
			DBAddress:  cfg.DBAddress,
			DBPort:     cfg.DBPort,
			DBType:     cfg.DBType,
			DBUserName: cfg.DBUserName,
			DBPassword: cfg.DBPassword,
		}
		w.s, err = storage.NewStorageDB(scfg)
		if err != nil {
			w.log.Error("Failed to configure storage DB", "err", err)
			return nil, err
		}
	default:
		return nil, fmt.Errorf("ivnalid wallet configuration, storgae type is not supported")
	}
	err = w.s.Init(DIDStorage, &DIDType{})
	if err != nil {
		w.log.Error("Failed to initialize DID storage", "err", err)
		return nil, err
	}
	err = w.s.Init(w.tokenStorage, &Token{})
	if err != nil {
		w.log.Error("Failed to initialize whole token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(w.partTokenStorage, &PartToken{})
	if err != nil {
		w.log.Error("Failed to initialize part token storage", "err", err)
		return nil, err
	}
	err = w.s.Init(w.creditStorage, &Credit{})
	if err != nil {
		w.log.Error("Failed to initialize credit storage", "err", err)
		return nil, err
	}
	return w, nil
}
