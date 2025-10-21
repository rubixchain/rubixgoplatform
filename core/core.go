package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/pubsub"
	"github.com/rubixchain/rubixgoplatform/core/service"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	didm "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/apiconfig"
	econfig "github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	APIPingPath                     string = "/api/ping"
	APIPeerStatus                   string = "/api/peerstatus"
	APICreditStatus                 string = "/api/creditstatus"
	APIQuorumConsensus              string = "/api/quorum-conensus"
	APIQuorumCredit                 string = "/api/quorum-credit"
	APIReqPledgeToken               string = "/api/req-pledge-token"
	APIUpdatePledgeToken            string = "/api/update-pledge-token"
	APISignatureRequest             string = "/api/signature-request"
	APISendReceiverToken            string = "/api/send-receiver-token"
	APIConfirmTokenTransfer         string = "/api/confirm-token-transfer"
	APIRollbackTransaction          string = "/api/rollback-transaction"
	APISyncTokenChain               string = "/api/sync-token-chain"
	APIDhtProviderCheck             string = "/api/dht-provider-check"
	APIMapDIDArbitration            string = "/api/map-did-arbitration"
	APICheckDIDArbitration          string = "/api/check-did-arbitration"
	APITokenArbitration             string = "/api/token-arbitration"
	APIGetTokenNumber               string = "/api/get-token-number"
	APIGetMigratedTokenStatus       string = "/api/get-Migrated-token-status"
	APISyncDIDArbitration           string = "/api/sync-did-arbitration"
	APIUnlockTokens                 string = "/api/unlock-tokens"
	APICheckQuorumStatusPath        string = "/api/check-quorum-status"
	APIGetPeerDIDTypePath           string = "/api/get-peer-didType"
	APIGetPeerInfoPath              string = "/api/get-peer-info"
	APIUpdateTokenHashDetails       string = "/api/update-tokenhash-details"
	APIAddUnpledgeDetails           string = "/api/initiate-unpledge"
	APISelfTransfer                 string = "/api/self-transfer"
	APIRecoverPinnedRBT             string = "/api/recover-pinned-rbt"
	APIRequestSigningHash           string = "/api/request-signing-hash"
	TokenValidatorURL               string = "http://103.209.145.177:8000"
	APISendFTToken                  string = "/api/send-ft-token"
	APIGetPrevQrmFromPrevSenderPath string = "/api/get-prev-qrms-info-from-sender"
	APICheckPinRole                 string = "/api/check-pin-role"
	APISyncGenesisAndLatestBlock    string = "/api/sync-gennesis-n-lastest-block"
	APIUpdateStatus                 string = "/api/update-status"
	APIGetTokenStatus               string = "/api/get-token-status"
	// APISendTokenChainDetails        string = "api/send-token-chain-details"
)

const (
	InvalidPasringErr        string = "invalid json parsing"
	RubixRootDir             string = "Rubix/"
	DefaultMainNetDB         string = "rubix.db"
	DefaultTestNetDB         string = "rubixtest.db"
	FullNodeMainNetDB        string = "fullnode-rubix.db"
	FullNodeTestNetDB        string = "fullnode-rubixtest.db"
	FullNodeTokensDBName     string = "fullnode_tokens_storage"
	FullNodeTestTokensDBName string = "fullnode_testtokens_storage"
	FullNodeTokensDBPort     string = "5432"
	MainNetDir               string = "MainNet"
	TestNetDir               string = "TestNet"
	TestNetDIDDir            string = "TestNetDID/"
	MaxDecimalPlaces         int    = 3
)

const (
	NodePort    uint16 = 20000
	SendPort    uint16 = 21000
	RecvPort    uint16 = 22000
	IPFSPort    uint16 = 5002
	SwarmPort   uint16 = 4002
	IPFSAPIPort uint16 = 8081
	MaxPeerConn uint16 = 1000
)

var dbWriteSem = make(chan struct{}, 1)

type Core struct {
	cfg                  *config.Config
	cfgFile              string
	encKey               string
	log                  logger.Logger
	peerID               string
	lock                 sync.RWMutex
	ipfsLock             sync.RWMutex
	qlock                sync.RWMutex
	rlock                sync.Mutex
	ipfs                 *ipfsnode.Shell
	ipfsState            bool
	ipfsChan             chan bool
	ipfsCmd              *exec.Cmd
	ipfsPID              int
	ipfsHealth           *IPFSHealthManager
	ipfsRecovery         *IPFSRecoveryManager
	ipfsOps              *IPFSOperations
	ipfsScalability      *IPFSScalabilityManager
	connRecovery         *ConnectionRecovery
	p2pReconnect         *P2PReconnectManager
	shutdownMgr          *ShutdownManager
	d                    *did.DID
	didDir               string
	pm                   *ipfsport.PeerManager
	qm                   *QuorumManager
	l                    *ipfsport.Listener
	ps                   *pubsub.PubSub
	started              bool
	ipfsApp              string
	testNet              bool
	testNetKey           string
	version              string
	quorumRequest        map[string]*ConsensusStatus
	pd                   map[string]*PledgeDetails
	webReq               map[string]*did.DIDChan
	w                    *wallet.Wallet
	qc                   map[string]did.DIDCrypto
	pqc                  map[string]did.DIDCrypto
	sd                   map[string]*ServiceDetials
	s                    storage.Storage
	fullNodeStorage      storage.Storage
	fullNodeTokensDB     storage.Storage
	as                   storage.Storage
	srv                  *service.Service
	arbitaryMode         bool
	arbitaryAddr         []string
	ec                   *ExplorerClient
	secret               []byte
	quorumCount          int
	noBalanceQuorumCount int
	defaultSetup         bool
	tokenSyncManager     *TokenSyncManager
	asyncPinManager      *AsyncPinManager
	perfTracker          *PerformanceTracker
	txStateMgr           *TransactionStateManager
	rollbackMgr          *RollbackManager
	tokenPool            *TokenInfoPool
	batchSyncTokenPool   *BatchSyncTokenInfoPool
	tokenSlicePool       *TokenSlicePool
	pendingTokenMonitor  *PendingTokenMonitor
	publishTokenChain    bool
	fullNode             bool
	txnProcessor         *DynamicTxnProcessor
	RetryTokenSyncTicker *time.Ticker
}

func InitConfig(configFile string, encKey string, node uint16, addr string) error {
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		nodePort := NodePort + node
		portOffset := MaxPeerConn * node
		cfg := config.Config{
			NodeAddress: addr,
			NodePort:    fmt.Sprintf("%d", nodePort),
			DirPath:     "./",
			CfgData: config.ConfigData{
				Ports: config.Ports{
					SendPort:     (SendPort + node),
					ReceiverPort: (RecvPort + portOffset),
					IPFSPort:     (IPFSPort + node),
					SwarmPort:    (SwarmPort + node),
					IPFSAPIPort:  (IPFSAPIPort + node),
				},
				BootStrap:     []string{"/ip4/161.35.169.251/tcp/4001/p2p/12D3KooWPhZEYEw4jG3kSRuwgMEHcVt7KMkm1ui2ddu4fgSgwvDq", "/ip4/103.127.158.120/tcp/4001/p2p/12D3KooWSQ94HRDzFf6W2rp7P8gzP6efZQHTaSU8uaQjskVBHiWP", "/ip4/172.104.191.191/tcp/4001/p2p/12D3KooWFudnWZY1v1m4YXCzDWZSbNt7nvf5F42uzM6vErZ4NwqJ"},
				TestBootStrap: []string{"/ip4/103.209.145.177/tcp/4001/p2p/12D3KooWD8Rw7Fwo4n7QdXTCjbh6fua8dTqjXBvorNz3bu7d9xMc", "/ip4/98.70.52.158/tcp/4001/p2p/12D3KooWQyWFABF3CKFnzX85hf5ZwrT5zPsy4rWHdGPZ8bBpRVCK", "/ip4/20.244.16.143/tcp/4001/p2p/12D3KooWAydFDJeSW5qupmp3AjRxc82Dq1AnjfJT1zwy4hg2TuNn", "/ip4/40.81.232.217/tcp/4001/p2p/12D3KooWK6V21GQotbub3cfgb5qAK1uUoUGPexf3vsLqw6yBJfen"},
			},
		}
		cfgBytes, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		err = apiconfig.CreateAPIConfig(configFile, encKey, cfgBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewCore(cfg *config.Config, cfgFile string, encKey string, log logger.Logger, testNet bool, testNetKey string, am bool, defaultSetup bool, publishTokenChainDetails bool, fullNode bool, fullnodeTokenDBUsername string, fullnodeTokenDBPassword string) (*Core, error) {
	var err error
	update := false
	if cfg.CfgData.StorageConfig.StorageType == 0 {
		cfg.CfgData.StorageConfig.StorageType = storage.StorageDBType
		cfg.CfgData.StorageConfig.DBAddress = cfg.DirPath + RubixRootDir + DefaultMainNetDB
		cfg.CfgData.StorageConfig.DBType = "Sqlite3"
		update = true
	}

	if cfg.CfgData.TestStorageConfig.StorageType == 0 {
		cfg.CfgData.TestStorageConfig.StorageType = storage.StorageDBType
		cfg.CfgData.TestStorageConfig.DBAddress = cfg.DirPath + RubixRootDir + DefaultTestNetDB
		cfg.CfgData.TestStorageConfig.DBType = "Sqlite3"
		update = true
	}

	c := &Core{
		cfg:               cfg,
		cfgFile:           cfgFile,
		encKey:            encKey,
		testNet:           testNet,
		testNetKey:        testNetKey,
		quorumRequest:     make(map[string]*ConsensusStatus),
		pd:                make(map[string]*PledgeDetails),
		webReq:            make(map[string]*did.DIDChan),
		qc:                make(map[string]did.DIDCrypto),
		pqc:               make(map[string]did.DIDCrypto),
		sd:                make(map[string]*ServiceDetials),
		arbitaryMode:      am,
		secret:            util.GetRandBytes(32),
		defaultSetup:      defaultSetup,
		publishTokenChain: publishTokenChainDetails,
		fullNode:          fullNode,
	}

	if c.fullNode {
		if c.testNet {
			if cfg.CfgData.FullnodeTestStorageConfig.StorageType == 0 {
				cfg.CfgData.FullnodeTestStorageConfig.StorageType = storage.StorageDBType
				cfg.CfgData.FullnodeTestStorageConfig.DBAddress = cfg.DirPath + RubixRootDir + FullNodeTestNetDB
				cfg.CfgData.FullnodeTestStorageConfig.DBType = "Sqlite3"
				update = true
			}
			if cfg.CfgData.FullnodeTestTokenStorageConfig.StorageType == 0 {
				cfg.CfgData.FullnodeTestTokenStorageConfig.StorageType = storage.StorageDBType
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBAddress = "localhost"
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBType = "PostgressSQL"
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBPort = FullNodeTokensDBPort
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBName = FullNodeTokensDBName
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBUserName = fullnodeTokenDBUsername
				cfg.CfgData.FullnodeTestTokenStorageConfig.DBPassword = fullnodeTokenDBPassword
				update = true
			}

		} else {

			if cfg.CfgData.FullnodeStorageConfig.StorageType == 0 {
				cfg.CfgData.FullnodeStorageConfig.StorageType = storage.StorageDBType
				cfg.CfgData.FullnodeStorageConfig.DBAddress = cfg.DirPath + RubixRootDir + FullNodeMainNetDB
				cfg.CfgData.FullnodeStorageConfig.DBType = "Sqlite3"
				update = true
			}

			if cfg.CfgData.FullnodeTokenStorageConfig.StorageType == 0 {
				cfg.CfgData.FullnodeTokenStorageConfig.StorageType = storage.StorageDBType
				cfg.CfgData.FullnodeTokenStorageConfig.DBAddress = "localhost"
				cfg.CfgData.FullnodeTokenStorageConfig.DBType = "PostgressSQL"
				cfg.CfgData.FullnodeTokenStorageConfig.DBPort = FullNodeTokensDBPort
				cfg.CfgData.FullnodeTokenStorageConfig.DBName = FullNodeTokensDBName
				cfg.CfgData.FullnodeTokenStorageConfig.DBUserName = fullnodeTokenDBUsername
				cfg.CfgData.FullnodeTokenStorageConfig.DBPassword = fullnodeTokenDBPassword
				update = true
			}
		}
	}

	c.didDir = c.cfg.DirPath + RubixRootDir
	if c.testNet {
		c.didDir = c.cfg.DirPath + RubixRootDir + TestNetDIDDir

	}
	if _, err := os.Stat(c.didDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create did directory", "err", err)
			return nil, err
		}
	}
	c.arbitaryAddr = []string{"12D3KooWHwsKu3GS9rh5X5eS9RTKGFy6NcdX1bV1UHcH8sQ8WqCM.bafybmicttgw2qx4grueyytrgln35vq2hbyhznv6ks4fabeakm47u72c26u",
		"12D3KooWQ2as3FNtvL1MKTeo7XAuBZxSv8QqobxX4AmURxyNe5mX.bafybmicro2m4kove5vsetej63xq4csobtlzchb2c34lp6dnakzkwtq2mmy",
		"12D3KooWJUJz2ipK78LAiwhc1QUVDvSMjZNBHt4vSAeVAq6FsneA.bafybmics43ef7ldgrogzurh7vukormpgscq4um44bss6mfuopsbjorbyaq",
		"12D3KooWC5fHUg2yzAHydgenodN52MYPKhpK4DKRfS8TSm3idSUV.bafybmif5qnkfnkkrffxvoofah3fjzkmieohjbgyte35rrjrn3goufaiykq",
		"12D3KooWDd7c7DAVb38a9vfCFpqxh5nHbDQ4CYjMJuFfBgzpiagK.bafybmie4iynumz2v3obbtkqirxrejjoljjs3l76frvl43wgalqqgprze6q"}

	c.log = log.Named("Core")

	c.ipfsChan = make(chan bool)

	if update {
		c.updateConfig()
	}
	if _, err := os.Stat(cfg.DirPath + RubixRootDir + MainNetDir); os.IsNotExist(err) {
		err := os.MkdirAll(cfg.DirPath+RubixRootDir+MainNetDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create main net directory", "err", err)
			return nil, err
		}
	}
	tcDir := cfg.DirPath + RubixRootDir + MainNetDir + "/"
	if testNet {
		if _, err := os.Stat(cfg.DirPath + RubixRootDir + TestNetDir); os.IsNotExist(err) {
			err := os.MkdirAll(cfg.DirPath+RubixRootDir+TestNetDir, os.ModeDir|os.ModePerm)
			if err != nil {
				c.log.Error("Failed to create test net directory", "err", err)
				return nil, err
			}
		}
		tcDir = cfg.DirPath + RubixRootDir + TestNetDir + "/"
	}

	sc := cfg.CfgData.StorageConfig
	if c.testNet {
		sc = cfg.CfgData.TestStorageConfig
	}

	switch sc.StorageType {

	case storage.StorageDBType:
		scfg := &econfig.Config{
			DBName:     sc.DBName,
			DBAddress:  sc.DBAddress,
			DBPort:     sc.DBPort,
			DBType:     sc.DBType,
			DBUserName: sc.DBUserName,
			DBPassword: sc.DBPassword,
		}
		c.s, err = storage.NewStorageDB(scfg)
		if err != nil {
			c.log.Error("Failed to create storage DB", "err", err)
			return nil, fmt.Errorf("failed to create storage DB")
		}
		if c.arbitaryMode {
			scfg.DBName = "ArbitaryDB"
			c.as, err = storage.NewStorageDB(scfg)
			if err != nil {
				c.log.Error("Failed to create storage DB", "err", err)
				return nil, fmt.Errorf("failed to create storage DB")
			}
		}
		if c.fullNode {
			fullNodeDBName := FullNodeMainNetDB
			if c.testNet {
				fullNodeDBName = FullNodeTestNetDB
			}
			fullNodeStoragecfg := &econfig.Config{
				DBAddress: cfg.DirPath + RubixRootDir + fullNodeDBName,
				DBType:    "Sqlite3",
				// Other fields like DBUserName, DBPassword, etc., can be copied from sc if needed, but defaults are fine for Sqlite3.
			}
			c.fullNodeStorage, err = storage.NewStorageDB(fullNodeStoragecfg)
			if err != nil {
				c.log.Error("Failed to create full node storage DB", "err", err)
				return nil, fmt.Errorf("failed to create full node storage DB")
			}

			// postgresql to store tokens
			fullNodePostgressDBName := FullNodeTokensDBName
			if c.testNet {
				fullNodePostgressDBName = FullNodeTestTokensDBName
			}
			fullNodeTokenStoragecfg := &econfig.Config{
				DBAddress:  "localhost",
				DBPort:     FullNodeTokensDBPort,
				DBName:     fullNodePostgressDBName,
				DBUserName: fullnodeTokenDBUsername,
				DBPassword: fullnodeTokenDBPassword,
				DBType:     "PostgressSQL",
			}
			c.fullNodeTokensDB, err = storage.NewStorageDB(fullNodeTokenStoragecfg)
			if err != nil {
				c.log.Error("Failed to create full node storage DB", "err", err)
				return nil, fmt.Errorf("failed to create full node storage DB")
			}
		}

	default:
		c.log.Error("Unsupported DB type, please check the configuration", "type", sc.StorageType)
		return nil, fmt.Errorf("unsupported DB type, please check the configuration")
	}

	c.w, err = wallet.InitWallet(c.s, c.fullNodeStorage, c.fullNodeTokensDB, tcDir, c.log, c.fullNode)
	if err != nil {
		c.log.Error("Failed to setup wallet", "err", err)
		return nil, err
	}
	c.qm, err = NewQuorumManager(c.s, c.log)
	if err != nil {
		c.log.Error("Failed to setup quorum manager", "err", err)
		return nil, err
	}
	if c.arbitaryMode {
		c.srv, err = service.NewService(c.s, c.as, c.log)
		if err != nil {
			c.log.Error("Failed to setup service", "err", err)
			return nil, err
		}
		c.log.Info("Arbitary mode is enabled")
	}
	err = c.InitRubixExplorer()
	if err != nil {
		c.log.Error("Failed to init explorer", "err", err)
		return nil, err
	}
	if c.testNet && c.defaultSetup {
		c.AddFaucetQuorums()
	}

	// Initialize token sync manager
	c.tokenSyncManager = NewTokenSyncManager(c.log)

	// Initialize async pin manager with 4 workers by default
	c.asyncPinManager = NewAsyncPinManager(c, 4)

	// Initialize performance tracker
	perfConfig := &PerformanceConfig{
		Enabled:        true, // TODO: Make this configurable
		DataPath:       c.cfg.DirPath,
		RetentionHours: 24,
		MaxFileSize:    100, // 100MB
		DetailLevel:    "detailed",
	}
	c.perfTracker, err = NewPerformanceTracker(perfConfig, c.log)
	if err != nil {
		c.log.Error("Failed to initialize performance tracker", "err", err)
		// Continue without performance tracking
		c.perfTracker = &PerformanceTracker{enabled: false}
	}

	// Initialize transaction state manager
	c.txStateMgr = NewTransactionStateManager(c)

	// Initialize rollback manager
	c.rollbackMgr = NewRollbackManager(c, c.txStateMgr)

	// Initialize token pools for memory optimization
	c.tokenPool = NewTokenInfoPool()
	c.batchSyncTokenPool = NewBatchSyncTokenInfoPool()
	c.tokenSlicePool = NewTokenSlicePool()

	// Initialize pending token monitor for self-healing
	// Check every 5 minutes for tokens pending > 10 minutes
	c.pendingTokenMonitor = NewPendingTokenMonitor(c, 5*time.Minute, 10*time.Minute)

	// Wrap storage with tracking if performance tracker is enabled
	if c.perfTracker != nil && c.perfTracker.enabled && c.s != nil {
		c.s = NewTrackedStorage(c.s, c)
		if c.arbitaryMode && c.as != nil {
			c.as = NewTrackedStorage(c.as, c)
		}
	}

	return c, nil
}

func (c *Core) getCoreAppName(peerID string) string {
	return peerID + "RubixCore"
}

// SetupCore will setup all core ports
func (c *Core) SetupCore() error {
	var err error
	c.log.Info("Setting up the core")
	cfg := &ipfsport.Config{AppName: c.getCoreAppName(c.peerID), Port: c.cfg.CfgData.Ports.ReceiverPort + 10}
	c.l, err = ipfsport.NewListener(cfg, c.log, c.ipfs)
	if err != nil {
		return err
	}
	bs := c.cfg.CfgData.BootStrap
	if c.testNet {
		bs = c.cfg.CfgData.TestBootStrap
	}
	c.pm = ipfsport.NewPeerManager(c.cfg.CfgData.Ports.ReceiverPort+11, c.cfg.CfgData.Ports.ReceiverPort+10, 5000, c.ipfs, c.log, bs, c.peerID)
	c.d = did.InitDID(c.didDir, c.log, c.ipfs)
	c.ps, err = pubsub.NewPubSub(c.ipfs, c.log)
	if err != nil {
		return err
	}
	err = c.initServices()
	if err != nil {
		c.log.Error("Failed to setup services", "err", err)
		return err
	}
	if c.fullNode {
		c.SubscribeTxnSetup()
	}
	c.w.SetupWallet(c.ipfs)
	// Set health-managed IPFS operations for the wallet
	if c.ipfsOps != nil {
		c.w.SetIPFSOperations(NewWalletIPFSAdapter(c.ipfsOps))
	}
	c.PingSetup()
	c.CheckQuorumStatusSetup()
	c.GetPeerdidTypeSetup()
	c.peerSetup()
	c.w.AddDIDLastChar()
	c.SetupToken()
	c.QuroumSetup()
	c.PinService()
	// c.RestartIncompleteTokenChainSyncs()
	//c.UnlockFTs()
	// c.selfTransferService()

	// Start token sync cleanup routine
	go c.tokenSyncCleanupRoutine()

	return nil
}

func (c *Core) GetStartStatus() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.started
}

func (c *Core) SetStartStatus() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.started = true
}

func (c *Core) Start() (bool, string) {
	if c.GetStartStatus() {
		return true, "Already Setup"
	}
	c.log.Info("Starting the core")

	err := c.l.Start()
	if err != nil {
		c.log.Error("failed to start ping port", "err", err)
		return false, "Failed to start ping port"
	}
	//c.w.ReleaseAllLockedTokens()
	// exp := model.ExploreModel{
	// 	Cmd:    ExpPeerStatusCmd,
	// 	PeerID: c.peerID,
	// 	Status: "On",
	// }
	// err = c.PublishExplorer(&exp)
	// if err != nil {
	// 	c.log.Error("Failed to publish message to explorer", "err", err)
	// 	return false, "Failed to publish message to explorer"
	// }
	// dt, err := c.w.GetAllDIDs()
	// if err == nil && len(dt) > 0 {
	// 	list := make([]string, 0)
	// 	for _, d := range dt {
	// 		list = append(list, d.DID)
	// 	}
	// 	// exp = model.ExploreModel{
	// 	// 	Cmd:     ExpDIDPeerMapCmd,
	// 	// 	PeerID:  c.peerID,
	// 	// 	DIDList: list,
	// 	// }
	// 	// err = c.PublishExplorer(&exp)
	// 	// if err != nil {
	// 	// 	c.log.Error("Failed to publish message to explorer", "err", err)
	// 	// 	return false, "Failed to publish message to explorer"
	// 	// }
	// }
	return true, "Setup Complete"
}

// TODO:: need to add more test
func (c *Core) NodeStatus() bool {
	return true
}

// IPFSOperations returns the IPFS operations wrapper
func (c *Core) IPFSOperations() *IPFSOperations {
	return c.ipfsOps
}

// GetIPFSStats returns IPFS health and scalability statistics
func (c *Core) GetIPFSStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if c.ipfsHealth != nil {
		stats["health"] = c.ipfsHealth.GetStats()
	}

	if c.ipfsScalability != nil {
		stats["scalability"] = c.ipfsScalability.GetScalabilityStats()
	}

	if c.ipfsRecovery != nil {
		stats["recovery"] = c.ipfsRecovery.GetRecoveryStats()
	}

	return stats
}

func (c *Core) StopCore() {
	// Initialize shutdown manager if not already done
	if c.shutdownMgr == nil {
		c.shutdownMgr = NewShutdownManager(c)
	}

	// stop retry-token-sync-ticker in case of fullnode
	if c.RetryTokenSyncTicker != nil {
		c.RetryTokenSyncTicker.Stop()
	}

	// Perform graceful shutdown
	if err := c.shutdownMgr.Shutdown(); err != nil {
		c.log.Error("Shutdown completed with errors", "error", err)
	} else {
		c.log.Info("Shutdown completed successfully")
	}
}

func (c *Core) CreateTempFolder() (string, error) {
	folderName := c.cfg.DirPath + "temp/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateSCTempFolder() (string, error) {
	folderName := c.cfg.DirPath + "SmartContract/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateNFTTempFolder() (string, error) {
	folderName := c.cfg.DirPath + "NFT/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) RenameSCFolder(tempFolderPath string, smartContractName string) (string, error) {
	scFolderName := filepath.Join(c.cfg.DirPath, "SmartContract", smartContractName)
	info, _ := os.Stat(scFolderName)

	// Check if the Smart Contract Folder exists
	if info == nil {
		// Directory not found, proceed to rename it
		err := os.Rename(tempFolderPath, scFolderName)
		if err != nil {
			c.log.Error("Unable to rename ", tempFolderPath, " to ", scFolderName, "error ", err)
			return "", err
		}
	}

	return scFolderName, nil
}

func (c *Core) RenameNFTFolder(tempFolderPath string, nft string) (string, error) {

	nftFolderName := c.cfg.DirPath + "NFT/" + nft
	err := os.Rename(tempFolderPath, nftFolderName)
	if err != nil {
		c.log.Error("Unable to rename ", tempFolderPath, " to ", nftFolderName, "error ", err)
		nftFolderName = ""
	}
	return nftFolderName, err
}

func (c *Core) HandleQuorum(conn net.Conn) {

}

func (c *Core) updateConfig() error {
	cfgBytes, err := json.Marshal(*c.cfg)
	if err != nil {
		c.log.Error("Failed to update config file", "err", err)
		return err
	}
	err = os.Remove(c.cfgFile)
	if err != nil {
		c.log.Error("Failed to update config file", "err", err)
		return err
	}
	err = apiconfig.CreateAPIConfig(c.cfgFile, c.encKey, cfgBytes)
	if err != nil {
		c.log.Error("Failed to update config file", "err", err)
		return err
	}
	return nil
}

// GetConfig returns the core configuration
func (c *Core) GetConfig() *config.Config {
	return c.cfg
}

func (c *Core) AddWebReq(req *ensweb.Request) {
	c.rlock.Lock()
	defer c.rlock.Unlock()
	c.webReq[req.ID] = &did.DIDChan{
		ID:      req.ID,
		InChan:  make(chan interface{}),
		OutChan: make(chan interface{}),
		Finish:  make(chan bool),
		Req:     req,
		Timeout: 3 * time.Minute,

		// Initialize password caching fields
		CachedPassword: "",
		PasswordSet:    false,
		PasswordMutex:  sync.RWMutex{},
	}
}

func (c *Core) GetWebReq(reqID string) *did.DIDChan {
	c.rlock.Lock()
	defer c.rlock.Unlock()
	req, ok := c.webReq[reqID]
	if !ok {
		return nil
	}
	return req
}

func (c *Core) UpateWebReq(reqID string, req *ensweb.Request) error {
	c.rlock.Lock()
	defer c.rlock.Unlock()
	dc, ok := c.webReq[reqID]
	if !ok {
		return fmt.Errorf("request does not exist")
	}
	dc.Req = req
	return nil
}

func (c *Core) RemoveWebReq(reqID string) *ensweb.Request {
	c.rlock.Lock()
	defer c.rlock.Unlock()
	req, ok := c.webReq[reqID]
	if !ok {
		return nil
	}

	// Clear cached password for security before removing request
	req.PasswordMutex.Lock()
	req.CachedPassword = ""
	req.PasswordSet = false
	req.PasswordMutex.Unlock()

	delete(c.webReq, reqID)
	return req.Req
}

func (c *Core) SetupDID(reqID string, didStr string) (did.DIDCrypto, error) {
	dt, err := c.w.GetDID(didStr)
	if err != nil {
		c.log.Error("DID does not exist", "did", didStr)
		return nil, fmt.Errorf("DID does not exist")
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return nil, fmt.Errorf("faield to get did channel")
	}
	switch dt.Type {
	case did.LiteDIDMode:
		return did.InitDIDLite(didStr, c.didDir, dc), nil
	case did.BasicDIDMode:
		return did.InitDIDBasic(didStr, c.didDir, dc), nil
	case did.StandardDIDMode:
		return did.InitDIDStandard(didStr, c.didDir, dc), nil
	case did.WalletDIDMode:
		return did.InitDIDWallet(didStr, c.didDir, dc), nil
	case did.ChildDIDMode:
		return did.InitDIDChild(didStr, c.didDir, dc), nil
	default:
		return nil, fmt.Errorf("DID Type is not supported")
	}
}

// Initializes the did in it's corresponding did mode (basic/ lite)
func (c *Core) SetupForienDID(didStr string, selfDID string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		c.log.Error("couldn't fetch did")
		return nil, err
	}

	// Fetching peer's did type
	peerInfo, err := c.GetPeerDIDInfo(didStr)
	if err != nil {
		if peerInfo == nil {
			c.log.Error("failed to get did type of peer did ", didStr, "error", err)
			return nil, err
		}
		if strings.Contains(err.Error(), "retry") {
			c.AddPeerDetails(*peerInfo)
		}
	}
	if peerInfo.DIDType == nil || *peerInfo.DIDType == -1 {
		c.log.Error("failed to get did type of peer did ", didStr, "error", err)
		return nil, err
	}

	return c.InitialiseDID(didStr, *peerInfo.DIDType)
}

// Initializes the quorum in it's corresponding did mode (basic/ lite)
func (c *Core) SetupForienDIDQuorum(didStr string, selfDID string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}

	// Fetching peer's did type
	peerInfo, err := c.GetPeerDIDInfo(didStr)
	if err != nil {
		if peerInfo == nil {
			c.log.Error("failed to get did type of peer did ", didStr, "error", err)
			return nil, err
		}
		if strings.Contains(err.Error(), "retry") {
			c.AddPeerDetails(*peerInfo)
		}
	}
	if *peerInfo.DIDType == -1 {
		c.log.Error("failed to get did type of peer did ", didStr, "error", err)
		return nil, err
	}

	switch *peerInfo.DIDType {
	case did.BasicDIDMode:
		return did.InitDIDQuorumc(didStr, c.didDir, ""), nil
	case did.LiteDIDMode:
		return did.InitDIDQuorumLite(didStr, c.didDir, ""), nil
	default:
		return did.InitDIDQuorumc(didStr, c.didDir, ""), nil
	}
}

func (c *Core) FetchDID(did string) error {
	didDir := c.didDir + did
	pubKeyPath := didDir + "/pubKey.pem"
	_, dirErr := os.Stat(didDir)
	_, pubKeyErr := os.Stat(pubKeyPath)

	if os.IsNotExist(dirErr) || os.IsNotExist(pubKeyErr) {
		// Directory or pubKey.pem missing, fetch from IPFS
		err := os.MkdirAll(didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("failed to create directory", "err", err)
			return err
		}
		err = c.ipfsOps.Get(did, didDir+"/")
		if err == nil {
			_, e := os.Stat(didDir + "/" + didm.MasterDIDFileName)
			// Fetch the master DID also
			if e == nil {
				var rb []byte
				rb, err = ioutil.ReadFile(didDir + "/" + didm.MasterDIDFileName)
				if err == nil {
					return c.FetchDID(string(rb))
				}
			}
		}
		return err
	}
	// Directory and pubKey.pem exist, nothing to do
	return nil
}

func (c *Core) GetNFTFromIpfs(nftTokenHash string, nftFolderHash string) error {
	dirPath := c.cfg.DirPath + "NFT/" + nftTokenHash
	// Check if the directory exists
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		// If the directory does not exist, create it
		err = os.MkdirAll(dirPath, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("failed to create directory", "err", err)
			return err
		}
	} else if err != nil {
		// Handle other errors while checking directory existence
		c.log.Error("failed to check directory existence", "err", err)
		return err
	}
	// Fetch NFT data from IPFS and store in the directory
	err = c.ipfsOps.Get(nftFolderHash, dirPath)
	if err != nil {
		c.log.Error("failed to get NFT from IPFS", "err", err)
		return err
	}
	c.log.Info("NFT data fetched successfully from ipfs")
	return nil
}

func (c *Core) GetPeerID() string {
	return c.peerID
}

// Initializes the did in it's corresponding did mode (basic/ lite)
func (c *Core) InitialiseDID(didStr string, didType int) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}
	switch didType {
	case did.LiteDIDMode:
		return did.InitDIDLite(didStr, c.didDir, nil), nil
	case did.BasicDIDMode:
		return did.InitDIDBasic(didStr, c.didDir, nil), nil
	default:
		return did.InitDIDBasic(didStr, c.didDir, nil), nil
	}
}

// StartPendingTokenMonitor starts the self-healing monitor for pending tokens
func (c *Core) StartPendingTokenMonitor() {
	if c.pendingTokenMonitor != nil {
		c.pendingTokenMonitor.Start()
		c.log.Info("Started pending token monitor for self-healing")
	}
}

// StopPendingTokenMonitor stops the pending token monitor
func (c *Core) StopPendingTokenMonitor() {
	if c.pendingTokenMonitor != nil {
		c.pendingTokenMonitor.Stop()
		c.log.Info("Stopped pending token monitor")
	}
}

// GetAsyncFTResponse returns the current value of the async FT response config flag
func (c *Core) GetAsyncFTResponse() bool {
	return c.cfg.CfgData.AsyncFTResponse
}

// SetAsyncFTResponse sets the async FT response config flag at runtime
func (c *Core) SetAsyncFTResponse(val bool) {
	c.cfg.CfgData.AsyncFTResponse = val
}

// tokenSyncCleanupRoutine periodically cleans up stale token sync entries
func (c *Core) tokenSyncCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.tokenSyncManager != nil {
				c.tokenSyncManager.CleanupStaleSync(10 * time.Minute)
			}
		}
	}
}

func (c *Core) RetryFailedTokenSync() {
	go func() {
		c.RetryTokenSyncTicker = time.NewTicker(1 * time.Minute)
		defer c.RetryTokenSyncTicker.Stop()

		for range c.RetryTokenSyncTicker.C {
			retryErrChan := make(chan error, 1)
			go func() {
				retryErrChan <- c.RetryFailedTOSyncTokens()
			}()

			err := <-retryErrChan
			if err != nil {
				c.log.Error("RetryFailedTOSyncTokens execution failed", "err", err)
			} else {
				c.log.Info("RetryFailedTOSyncTokens executed successfully")
			}

		}

	}()
}
