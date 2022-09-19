package wallet

import (
	"fmt"
	"sync"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	TokenStorage     string = "Tokens"
	PartTokenStorage string = "PartTokens"
)

type WalletConfig struct {
	StorageType int    `json:"stroage_type"`
	DBName      string `json:"db_name"`
	DBAddress   string `json:"db_address"`
	DBPort      string `json:"db_port"`
	DBType      string `json:"db_type"`
	DBUserName  string `json:"db_user_name"`
	DBPassword  string `json:"db_password"`
}

type Wallet struct {
	s   storage.Storage
	l   sync.Mutex
	log logger.Logger
}

func InitWallet(cfg *WalletConfig, log logger.Logger) (*Wallet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("invalid wallet configuration")
	}
	var err error
	w := &Wallet{
		log: log,
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
	return w, nil
}
