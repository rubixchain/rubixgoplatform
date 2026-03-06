package core

import (
	"context"
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
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/pubsub"
	"github.com/rubixchain/rubixgoplatform/core/service"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	didm "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/apiconfig"
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
	LocalNetDir              string = "LocalNet"
	TestNetDIDDir            string = "TestNetDID/"
	LocalNetDIDDir           string = "LocalNetDID/"
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
	testnet              bool
	testNetKey           string
	version              string
	quorumRequest        map[string]*ConsensusStatus
	pd                   map[string]*PledgeDetails
	webReq               map[string]*did.DIDChan
	w                    *wallet.Wallet
	qc                   map[string]did.DIDCrypto
	pqc                  map[string]did.DIDCrypto
	sd                   map[string]*ServiceDetials
	srv                  *service.Service
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
	faucetURL            string // for faucet url
	mainnet              bool
	localnet             bool
	s                    *storage.RubixDB
}

func InitConfig(configFile string, encKey string, node uint16, addr string) error {
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		nodePort := NodePort + node
		portOffset := MaxPeerConn * node
		cfg := config.Config{
			NodeAddress:   addr,
			NodePort:      fmt.Sprintf("%d", nodePort),
			NodeConfigDir: "./",
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

func NewCore(cfg *config.Config, dbConfig *types.DBConfig, cfgFile string, encKey string, log logger.Logger,
	networkMode string, testNetKey string, defaultSetup bool, publishTokenChainDetails bool,
	fullNode bool, faucetURL string,
) (*Core, error) {
	var err error
	update := true

	c := &Core{
		cfg:               cfg,
		cfgFile:           cfgFile,
		encKey:            encKey,
		testNetKey:        testNetKey,
		quorumRequest:     make(map[string]*ConsensusStatus),
		pd:                make(map[string]*PledgeDetails),
		webReq:            make(map[string]*did.DIDChan),
		qc:                make(map[string]did.DIDCrypto),
		pqc:               make(map[string]did.DIDCrypto),
		sd:                make(map[string]*ServiceDetials),
		secret:            util.GetRandBytes(32),
		defaultSetup:      defaultSetup,
		publishTokenChain: publishTokenChainDetails,
		fullNode:          fullNode,
		faucetURL:         faucetURL,
	}

	switch networkMode {
	case constants.NetworkMode_Testnet:
		c.testnet = true
	case constants.NetworkMode_Mainnet:
		c.mainnet = true
	case constants.NetworkMode_Local:
		c.localnet = true
	default:
		errMsg := fmt.Sprintf("Invalid network mode: %s", networkMode)
		return nil, fmt.Errorf(errMsg)
	}

	var tcDir string

	if c.mainnet {
		c.cfg.CfgData.StorageConfig.StorageType = dbConfig.DBType
		c.cfg.CfgData.StorageConfig.DBAddress = dbConfig.DBAddress
		c.cfg.CfgData.StorageConfig.DBType = dbConfig.DBType
		c.cfg.CfgData.StorageConfig.DBName = dbConfig.DBName
		c.cfg.CfgData.StorageConfig.DBUserName = dbConfig.DBUserName
		c.cfg.CfgData.StorageConfig.DBPassword = dbConfig.DBPassword
		c.cfg.CfgData.StorageConfig.DBPort = dbConfig.DBPort

		c.didDir = c.cfg.NodeConfigDir + RubixRootDir

		//TODO: To be removed since LevelDB is not in use
		tcDir = c.cfg.NodeConfigDir + RubixRootDir + MainNetDir + "/"
		if _, err := os.Stat(tcDir); os.IsNotExist(err) {
			err := os.MkdirAll(tcDir, os.ModeDir|os.ModePerm)
			if err != nil {
				c.log.Error("Failed to create main net directory", "err", err)
				return nil, err
			}
		}
	}

	if c.testnet {
		c.cfg.CfgData.TestStorageConfig.StorageType = dbConfig.DBType
		c.cfg.CfgData.TestStorageConfig.DBAddress = dbConfig.DBAddress
		c.cfg.CfgData.TestStorageConfig.DBType = dbConfig.DBType
		c.cfg.CfgData.TestStorageConfig.DBName = dbConfig.DBName
		c.cfg.CfgData.TestStorageConfig.DBUserName = dbConfig.DBUserName
		c.cfg.CfgData.TestStorageConfig.DBPassword = dbConfig.DBPassword
		c.cfg.CfgData.TestStorageConfig.DBPort = dbConfig.DBPort

		c.didDir = c.cfg.NodeConfigDir + RubixRootDir + TestNetDIDDir

		tcDir = c.cfg.NodeConfigDir + RubixRootDir + TestNetDir + "/"
		if _, err := os.Stat(tcDir); os.IsNotExist(err) {
			err := os.MkdirAll(tcDir, os.ModeDir|os.ModePerm)
			if err != nil {
				c.log.Error("Failed to create test net directory", "err", err)
				return nil, err
			}
		}
	}

	if c.localnet {
		c.cfg.CfgData.LocalStorageConfig.StorageType = dbConfig.DBType
		c.cfg.CfgData.LocalStorageConfig.DBAddress = dbConfig.DBAddress
		c.cfg.CfgData.LocalStorageConfig.DBType = dbConfig.DBType
		c.cfg.CfgData.LocalStorageConfig.DBName = dbConfig.DBName
		c.cfg.CfgData.LocalStorageConfig.DBUserName = dbConfig.DBUserName
		c.cfg.CfgData.LocalStorageConfig.DBPassword = dbConfig.DBPassword
		c.cfg.CfgData.LocalStorageConfig.DBPort = dbConfig.DBPort

		c.didDir = c.cfg.NodeConfigDir + RubixRootDir + LocalNetDIDDir

		tcDir = c.cfg.NodeConfigDir + RubixRootDir + LocalNetDir + "/"
		if _, err := os.Stat(tcDir); os.IsNotExist(err) {
			err := os.MkdirAll(tcDir, os.ModeDir|os.ModePerm)
			if err != nil {
				c.log.Error("Failed to create local net directory", "err", err)
				return nil, err
			}
		}
	}

	if _, err := os.Stat(c.didDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create did directory", "err", err)
			return nil, err
		}
	}

	c.log = log.Named("Core")
	c.ipfsChan = make(chan bool)

	if update {
		c.updateConfig()
	}

	rubixContext := context.Background()

	rubixDB, err := storage.NewRubixDB(rubixContext, dbConfig, storage.DefaultPoolOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to define Rubix DB: %v", err)
	}

	c.w, err = wallet.NewWallet(rubixContext, rubixDB, c.log)
	if err != nil {
		c.log.Error("Failed to setup wallet", "err", err)
		return nil, err
	}

	c.qm, err = NewQuorumManager(rubixDB, c.log)
	if err != nil {
		c.log.Error("Failed to setup quorum manager", "err", err)
		return nil, err
	}

	if c.testnet && c.defaultSetup {
		c.AddDefaulTestnetQuorums()
	}

	// Initialize token sync manager
	c.tokenSyncManager = NewTokenSyncManager(c.log)

	// Initialize async pin manager with 4 workers by default
	c.asyncPinManager = NewAsyncPinManager(c, 4)

	// Initialize performance tracker
	perfConfig := &PerformanceConfig{
		Enabled:        true, // TODO: Make this configurable
		DataPath:       c.cfg.NodeConfigDir,
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
	// TODO: update the following
	// if c.perfTracker != nil && c.perfTracker.enabled && c.s != nil {
	// 	c.s = NewTrackedStorage(*rubixDB, c)
	// }

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
	if c.testnet || c.localnet {
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
	c.peerSetup()
	c.removePeerSetup()
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
	folderName := c.cfg.NodeConfigDir + "temp/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateSCTempFolder() (string, error) {
	folderName := c.cfg.NodeConfigDir + "SmartContract/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateNFTTempFolder() (string, error) {
	folderName := c.cfg.NodeConfigDir + "NFT/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) RenameSCFolder(tempFolderPath string, smartContractName string) (string, error) {
	scFolderName := filepath.Join(c.cfg.NodeConfigDir, "SmartContract", smartContractName)
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

	nftFolderName := c.cfg.NodeConfigDir + "NFT/" + nft
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
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return nil, fmt.Errorf("faield to get did channel")
	}
	return did.InitDIDLite(didStr, c.didDir, dc), nil
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
	return c.InitialiseDID(didStr)
}

// Initializes the quorum in it's corresponding did mode (basic/ lite)
func (c *Core) SetupForienDIDQuorum(didStr string, selfDID string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}
	return did.InitDIDQuorumLite(didStr, c.didDir, ""), nil
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
	dirPath := c.cfg.NodeConfigDir + "NFT/" + nftTokenHash
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

// Initializes the DID in it's corresponding did mode
func (c *Core) InitialiseDID(didStr string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}
	return did.InitDIDLite(didStr, c.didDir, nil), nil
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
		c.RetryTokenSyncTicker = time.NewTicker(1 * time.Hour)
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
