package wallet

import (
	"fmt"
	"sync"

	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	TokenStorage       string = "TokensTable"
	DataTokenStorage   string = "DataTokensTable"
	NFTTokenStorage    string = "NFTTokensTable"
	CreditStorage      string = "CreditsTable"
	DIDStorage         string = "DIDTable"
	DIDPeerStorage_a   string = "DIDPeerTable_a"
	DIDPeerStorage_b   string = "DIDPeerTable_b"
	DIDPeerStorage_c   string = "DIDPeerTable_c"
	DIDPeerStorage_d   string = "DIDPeerTable_d"
	DIDPeerStorage_e   string = "DIDPeerTable_e"
	DIDPeerStorage_f   string = "DIDPeerTable_f"
	DIDPeerStorage_g   string = "DIDPeerTable_g"
	DIDPeerStorage_h   string = "DIDPeerTable_h"
	DIDPeerStorage_i   string = "DIDPeerTable_i"
	DIDPeerStorage_j   string = "DIDPeerTable_j"
	DIDPeerStorage_k   string = "DIDPeerTable_k"
	DIDPeerStorage_l   string = "DIDPeerTable_l"
	DIDPeerStorage_m   string = "DIDPeerTable_m"
	DIDPeerStorage_n   string = "DIDPeerTable_n"
	DIDPeerStorage_o   string = "DIDPeerTable_o"
	DIDPeerStorage_p   string = "DIDPeerTable_p"
	DIDPeerStorage_q   string = "DIDPeerTable_q"
	DIDPeerStorage_r   string = "DIDPeerTable_r"
	DIDPeerStorage_s   string = "DIDPeerTable_s"
	DIDPeerStorage_t   string = "DIDPeerTable_t"
	DIDPeerStorage_u   string = "DIDPeerTable_u"
	DIDPeerStorage_v   string = "DIDPeerTable_v"
	DIDPeerStorage_w   string = "DIDPeerTable_w"
	DIDPeerStorage_x   string = "DIDPeerTable_x"
	DIDPeerStorage_y   string = "DIDPeerTable_y"
	DIDPeerStorage_z   string = "DIDPeerTable_z"
	DIDPeerStorage_0   string = "DIDPeerTable_0"
	DIDPeerStorage_1   string = "DIDPeerTable_1"
	DIDPeerStorage_2   string = "DIDPeerTable_2"
	DIDPeerStorage_3   string = "DIDPeerTable_3"
	DIDPeerStorage_4   string = "DIDPeerTable_4"
	DIDPeerStorage_5   string = "DIDPeerTable_5"
	DIDPeerStorage_6   string = "DIDPeerTable_6"
	DIDPeerStorage_7   string = "DIDPeerTable_7"
	DIDPeerStorage_8   string = "DIDPeerTable_8"
	DIDPeerStorage_9   string = "DIDPeerTable_9"
	TransactionStorage string = "TransactionHistory"
	TokensArrayStorage string = "TokensTransferred"
	TokenProvider      string = "TokenProviderTable"
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
	op := &opt.Options{
		WriteBuffer: 64 * 1024 * 1024,
	}

	tdb, err := leveldb.OpenFile(dir+TokenChainStorage, op)
	if err != nil {
		w.log.Error("failed to configure token chain block storage", "err", err)
		return nil, fmt.Errorf("failed to configure token chain block storage")
	}
	w.tcs.DB = *tdb
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
	err = w.s.Init(CreditStorage, &Credit{}, true)
	if err != nil {
		w.log.Error("Failed to initialize credit storage", "err", err)
		return nil, err
	}

	err = w.s.Init(DIDPeerStorage_a, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `a`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_b, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `b`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_c, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `c`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_d, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `d`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_e, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `e`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_f, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `f`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_g, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `g`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_h, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `h`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_i, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `i`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_j, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `j`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_k, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `k`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_l, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `l`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_m, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `m`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_n, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `n`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_o, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `o`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_p, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `p`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_q, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `q`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_r, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `r`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_s, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `s`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_t, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `t`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_u, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `u`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_v, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `v`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_w, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `w`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_x, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `x`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_y, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `y`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_z, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `z`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_0, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `0`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_1, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `1`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_2, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `2`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_3, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `3`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_4, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `4`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_5, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `5`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_6, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `6`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_7, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `7`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_8, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `8`", "err", err)
		return nil, err
	}
	err = w.s.Init(DIDPeerStorage_9, &DIDPeerMap{}, true)
	if err != nil {
		w.log.Error("Failed to initialize DID Peer storage of `9`", "err", err)
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
	return w, nil
}

func (w *Wallet) SetupWallet(ipfs *ipfsnode.Shell) {
	w.ipfs = ipfs
}
