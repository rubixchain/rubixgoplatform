package core

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/api"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/service"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	APIPingPath                        string = "/api/ping"
	APIPeerStatus                      string = "/api/peerstatus"
	APICreditStatus                    string = "/api/creditstatus"
	APIQuorumConsensus                 string = "/api/quorum-conensus"
	APIQuorumCredit                    string = "/api/quorum-credit"
	APIReqPledgeToken                  string = "/api/req-pledge-token"
	APIUpdatePledgeToken               string = "/api/update-pledge-token"
	APISignatureRequest                string = "/api/signature-request"
	APISendReceiverToken               string = "/api/send-receiver-token"
	APIConfirmTokenTransfer            string = "/api/confirm-token-transfer"
	APIRollbackTransaction             string = "/api/rollback-transaction"
	APISyncTokenChain                  string = "/api/sync-token-chain"
	APISyncTransactionChain            string = "/api/sync-transaction-chain"
	APIDhtProviderCheck                string = "/api/dht-provider-check"
	APIMapDIDArbitration               string = "/api/map-did-arbitration"
	APICheckDIDArbitration             string = "/api/check-did-arbitration"
	APITokenArbitration                string = "/api/token-arbitration"
	APIGetTokenNumber                  string = "/api/get-token-number"
	APIGetMigratedTokenStatus          string = "/api/get-Migrated-token-status"
	APISyncDIDArbitration              string = "/api/sync-did-arbitration"
	APIUnlockTokens                    string = "/api/unlock-tokens"
	APICheckQuorumStatusPath           string = "/api/check-quorum-status"
	APIGetPeerInfoPath                 string = "/api/get-peer-info"
	APIUpdateTokenHashDetails          string = "/api/update-tokenhash-details"
	APIAddUnpledgeDetails              string = "/api/initiate-unpledge"
	APISelfTransfer                    string = "/api/self-transfer"
	TokenValidatorURL                  string = "http://103.209.145.177:8000"
	APISendFTToken                     string = "/api/send-ft-token"
	APIGetPrevQrmFromPrevSenderPath    string = "/api/get-prev-qrms-info-from-sender"
	APICheckPinRole                    string = "/api/check-pin-role"
	APISyncGenesisAndLatestBlock       string = "/api/sync-gennesis-n-lastest-block"
	APISyncGenesisAndLatestTransaction string = "/api/sync-genesis-n-lastest-transaction"
	APIUpdateStatus                    string = "/api/update-status"
	APIGetTokenStatus                  string = "/api/get-token-status"
	APIInitiateConsensus               string = "/api/initiate-consensus"
	APISendTokens                      string = "/api/send-tokens"
	APIRequestPledgeToken              string = "/api/request-pledge-token"
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
	FullNodeTokensDBPort     string = "5500"
	MainNetDir               string = "MainNet"
	TestNetDir               string = "TestNet"
	LocalNetDir              string = "LocalNet"
	TestNetDIDDir            string = "TestNetDID/"
	LocalNetDIDDir           string = "LocalNetDID/"
	MaxDecimalPlaces         int    = 3
)

var dbWriteSem = make(chan struct{}, 1)

type Core struct {
	cfg                  *types.RubixConfig
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
	l                    *ipfsport.Listener
	ps                   *types.PubSub
	started              bool
	ipfsApp              string
	testnet              bool
	version              string
	quorumRequest        map[string]*ConsensusStatus
	pd                   map[string]*PledgeDetails
	webReq               map[string]*did.DIDChan
	w                    *wallet.Wallet
	qc                   map[string]types.DIDCrypto
	pqc                  map[string]types.DIDCrypto
	secret               []byte
	quorumCount          int
	noBalanceQuorumCount int
	defaultSetup         bool
	tokenSyncManager     *TokenSyncManager
	ipfsProviderStore    *IPFSProviderStore
	asyncPinManager      *AsyncPinManager
	perfTracker          *PerformanceTracker
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
	Ctx                  context.Context
	qm                   *QuorumManager
	srv                  *service.Service
}

func newRubixContext() context.Context {
	return context.Background()
}

func NewCore(cfg *types.RubixConfig, log logger.Logger,
	networkMode string, defaultSetup bool, publishTokenChainDetails bool,
	fullNode bool, faucetURL string,
) (*Core, error) {
	var err error

	c := &Core{
		cfg:               cfg,
		quorumRequest:     make(map[string]*ConsensusStatus),
		pd:                make(map[string]*PledgeDetails),
		webReq:            make(map[string]*did.DIDChan),
		qc:                make(map[string]types.DIDCrypto),
		pqc:               make(map[string]types.DIDCrypto),
		secret:            func() []byte { b := make([]byte, 32); cryptorand.Read(b); return b }(),
		defaultSetup:      defaultSetup,
		publishTokenChain: publishTokenChainDetails,
		fullNode:          fullNode,
		faucetURL:         faucetURL,
		Ctx:               newRubixContext(),
		qm:                &QuorumManager{},
		srv:               service.New(),
	}

	switch networkMode {
	case constants.NetworkMode_Testnet:
		c.testnet = true
	case constants.NetworkMode_Mainnet:
		c.mainnet = true
	case constants.NetworkMode_Localnet:
		c.localnet = true
	default:
		errMsg := fmt.Sprintf("Invalid network mode: %s", networkMode)
		return nil, fmt.Errorf(errMsg)
	}

	c.log = log.Named("Core")
	c.didDir = c.cfg.DidDir

	if _, err := os.Stat(c.didDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create did directory", "err", err)
			return nil, err
		}
	}
	c.ipfsChan = make(chan bool)

	dbOpts := storage.DBOpts{
		MaxConns:                  c.cfg.DBConfig.Params.MaxConnections,
		MinConns:                  c.cfg.DBConfig.Params.MinConnections,
		MaxConnLifetimeInSeconds:  time.Duration(c.cfg.DBConfig.Params.MaxConnectionLifetimeSeconds) * time.Second,
		MaxConnIdleTimeInSeconds:  time.Duration(c.cfg.DBConfig.Params.MaxConnectionIdletimeSeconds) * time.Second,
		StatementTimeoutInSeconds: time.Duration(c.cfg.DBConfig.Params.StatementTimeoutSeconds) * time.Second,
	}

	rubixDB, err := storage.NewRubixDB(c.Ctx, &c.cfg.DBConfig, dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to define Rubix DB: %v", err)
	}

	c.w, err = wallet.NewWallet(c.Ctx, rubixDB, c.log)
	if err != nil {
		c.log.Error("Failed to setup wallet", "err", err)
		return nil, err
	}

	c.ipfsProviderStore = NewIPFSProviderStore(rubixDB.Pool(), c.log, func() string {
		return c.peerID
	})

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
		DataPath:       c.cfg.NodeDir,
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
	c.didDir = c.cfg.DidDir

	cfg := &ipfsport.Config{AppName: c.getCoreAppName(c.peerID), Port: c.cfg.PortConfig.ReceiverPort + 10}
	c.l, err = ipfsport.NewListener(cfg, c.log, c.ipfs)
	if err != nil {
		return err
	}

	bs := c.cfg.MainnetBootstrap
	if c.testnet {
		bs = c.cfg.TestnetBootstrap
	}
	if c.localnet {
		bs = c.cfg.LocalnetBootStrap
	}

	c.pm = ipfsport.NewPeerManager(c.cfg.PortConfig.ReceiverPort+11, c.cfg.PortConfig.ReceiverPort+10, 5000, c.ipfs, c.log, bs, c.peerID)
	c.d = did.InitDID(c.didDir, c.log, c.ipfs)
	c.ps, err = types.NewPubSub(c.ipfs, c.log)
	if err != nil {
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
	c.SetupToken()
	c.QuroumSetup()
	c.PinService()
	api.SetupAPI(c.l, c.w, c.log)

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
	folderName := path.Join(c.cfg.NetworkDir, "temp", uuid.New().String())
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateSCTempFolder() (string, error) {
	folderName := path.Join(c.cfg.NetworkDir, "smart_contracts", uuid.New().String())
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateNFTTempFolder() (string, error) {
	folderName := path.Join(c.cfg.NetworkDir, "nfts", uuid.New().String())
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) RenameSCFolder(tempFolderPath string, smartContractName string) (string, error) {
	scFolderName := path.Join(c.cfg.NetworkDir, "smart_contracts", smartContractName)
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
	nftFolderName := path.Join(c.cfg.NetworkDir, "nfts", nft)
	err := os.Rename(tempFolderPath, nftFolderName)
	if err != nil {
		c.log.Error("Unable to rename ", tempFolderPath, " to ", nftFolderName, "error ", err)
		nftFolderName = ""
	}
	return nftFolderName, err
}

// GetConfig returns the core configuration
func (c *Core) GetConfig() *types.RubixConfig {
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

func (c *Core) SetupDID(reqID string, didStr string) (types.DIDCrypto, error) {
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return nil, fmt.Errorf("faield to get did channel")
	}
	return did.InitDIDLite(didStr, c.didDir, dc), nil
}

// Initializes the did in it's corresponding did mode (basic/ lite)
func (c *Core) SetupForienDID(didStr string, selfDID string) (types.DIDCrypto, error) {
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
func (c *Core) SetupForienDIDQuorum(didStr string, selfDID string) (types.DIDCrypto, error) {
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
			c.log.Error("failed to perform ipfs get on input did", "err", err)
			return err
		}
		return err
	}
	// Directory and pubKey.pem exist, nothing to do
	return nil
}

func (c *Core) GetNFTFromIpfs(nftTokenHash string, nftFolderHash string) error {
	dirPath := path.Join(c.cfg.NetworkDir, "nfts", nftTokenHash)
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
func (c *Core) InitialiseDID(didStr string) (types.DIDCrypto, error) {
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
	return c.cfg.AsyncFTResponse
}

// SetAsyncFTResponse sets the async FT response config flag at runtime
func (c *Core) SetAsyncFTResponse(val bool) {
	c.cfg.AsyncFTResponse = val
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
