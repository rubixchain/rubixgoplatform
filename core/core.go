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
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	APIPingPath              string = "/rubix/v1/internal/ping"
	APIPeerStatus            string = "/rubix/v1/internal/peer_status"
	APISyncTransactionChain  string = "/rubix/v1/internal/sync_transaction_chain"
	APIFetchGenesisTxn       string = "/rubix/v1/internal/fetch_genesis_transaction"
	APICheckQuorumStatusPath string = "/rubix/v1/internal/quorum_status"
	APIGetPeerInfoPath       string = "/rubix/v1/internal/peer_info"
	APIInitiateConsensus     string = "/rubix/v1/internal/initiate_consensus"
	APISendTokens            string = "/rubix/v1/internal/send_tokens"
	APIRequestPledgeToken    string = "/rubix/v1/internal/request_pledge_token"
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

type Core struct {
	cfg                  *types.RubixConfig
	log                  logger.Logger
	peerID               string
	lock                 sync.RWMutex
	ipfsLock             sync.RWMutex
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
	nftDir               string
	smartContractDir     string
	pm                   *ipfsport.PeerManager
	l                    *ipfsport.Listener
	ps                   *types.PubSub
	started              bool
	ipfsApp              string
	testnet              bool
	networkMode          string
	version              string
	webReq               map[string]*did.DIDChan
	w                    *wallet.Wallet
	qc                   map[string]types.DIDCrypto
	pqc                  map[string]types.DIDCrypto
	secret               []byte
	ipfsProviderStore    *IPFSProviderStore
	perfTracker          *PerformanceTracker
	fullNode             bool
	txnProcessor         *DynamicTxnProcessor
	RetryTokenSyncTicker *time.Ticker
	faucetURL            string // for faucet url
	mainnet              bool
	localnet             bool
	Ctx                  context.Context
	// Unpledge mismatch audit log — lazy-init on first mismatch event.
	// See core/unpledge_v2.go:writeUnpledgeMismatch.
	unpledgeAuditLog     *os.File
	unpledgeAuditLogOnce sync.Once
	unpledgeAuditLogMu   sync.Mutex
	// recoverySessions holds the live, single-use ownership-proof nonces issued
	// to recovering nodes. Populated only on a fullnode (initialised in
	// registerRecoveryRoute). See core/fullnode_recovery.go.
	recoverySessions *recoverySessionStore
}

func newRubixContext() context.Context {
	return context.Background()
}

func NewCore(cfg *types.RubixConfig, log logger.Logger,
	networkMode string,
	fullNode bool, faucetURL string,
) (*Core, error) {
	var err error

	c := &Core{
		cfg:       cfg,
		webReq:    make(map[string]*did.DIDChan),
		qc:        make(map[string]types.DIDCrypto),
		pqc:       make(map[string]types.DIDCrypto),
		secret:    func() []byte { b := make([]byte, 32); cryptorand.Read(b); return b }(),
		fullNode:  fullNode,
		faucetURL: faucetURL,
		Ctx:       newRubixContext(),
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
	c.networkMode = networkMode

	c.log = log.Named("Core")
	c.didDir = c.cfg.DidDir
	c.nftDir = c.cfg.NFTDir
	c.smartContractDir = c.cfg.SmartContractDir

	if _, err := os.Stat(c.didDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create did directory", "err", err)
			return nil, err
		}
	}

	if _, err := os.Stat(c.nftDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.nftDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create nft directory", "err", err)
			return nil, err
		}
	}

	if _, err := os.Stat(c.smartContractDir); os.IsNotExist(err) {
		err := os.MkdirAll(c.smartContractDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create smart contract directory", "err", err)
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
	c.w.SetDidDir(c.didDir)

	c.ipfsProviderStore = NewIPFSProviderStore(rubixDB.Pool(), c.log, func() string {
		return c.peerID
	})

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
	c.nftDir = c.cfg.NFTDir
	c.smartContractDir = c.cfg.SmartContractDir

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
		// Fullnodes hold an authoritative dids table and serve DID->peerID
		// lookups for nodes that missed the ephemeral rubix_did announcement.
		if err := c.peerInfoResponderSetup(); err != nil {
			c.log.Error("Failed to subscribe to peer_info responder topic", "err", err)
		}
	}

	c.w.SetupWallet(c.ipfs)
	// Set health-managed IPFS operations for the wallet
	if c.ipfsOps != nil {
		c.w.SetIPFSOperations(NewWalletIPFSAdapter(c.ipfsOps))
	}
	c.PingSetup()
	c.CheckQuorumStatusSetup()
	c.peerSetup()
	c.QuorumSetup()
	c.TransactionSetup()

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
	return true, "Setup Complete"
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
	folderName := path.Join(c.smartContractDir, uuid.New().String())
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) CreateNFTTempFolder() (string, error) {
	folderName := path.Join(c.nftDir, uuid.New().String())
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
}

func (c *Core) RenameSCFolder(tempFolderPath string, smartContractName string) (string, error) {
	scFolderName := path.Join(c.smartContractDir, smartContractName)
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
	nftFolderName := path.Join(c.nftDir, nft)
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

// GetWallet returns the wallet instance for direct access in dev/test scenarios.
// In production, wallet operations go through Core methods.
func (c *Core) GetWallet() *wallet.Wallet {
	return c.w
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
	didDir := path.Join(c.didDir, did)

	pubKeyPath := path.Join(didDir, constants.PubKeyFileName)
	_, dirErr := os.Stat(didDir)
	_, pubKeyErr := os.Stat(pubKeyPath)

	if os.IsNotExist(dirErr) || os.IsNotExist(pubKeyErr) {
		err := os.MkdirAll(didDir, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("failed to create directory", "err", err)
			return err
		}
		type result struct{ err error }
		ch := make(chan result, 1)
		go func() {
			ch <- result{c.ipfsOps.Get(did, didDir+"/")}
		}()

		select {
		case r := <-ch:
			if r.err != nil {
				c.log.Error("failed to perform ipfs get on input did", "did", did, "err", r.err)
				return r.err
			}
			return nil
		case <-time.After(2 * time.Minute):
			return fmt.Errorf("FetchDID: timed out after 2 minutes fetching DID %s from IPFS", did)
		}
	}
	return nil
}

func (c *Core) GetNFTFromIpfs(nftTokenHash string, nftFolderHash string) error {
	dirPath := path.Join(c.nftDir, nftTokenHash)
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

// GetAsyncFTResponse returns the current value of the async FT response config flag
func (c *Core) GetAsyncFTResponse() bool {
	return c.cfg.AsyncFTResponse
}

// SetAsyncFTResponse sets the async FT response config flag at runtime
func (c *Core) SetAsyncFTResponse(val bool) {
	c.cfg.AsyncFTResponse = val
}

// GetSyncTransactionChainData returns transaction chains for the given token IDs,
// excluding any transactions whose IDs appear in excludeTxIDs.
func (c *Core) GetSyncTransactionChainData(tokenIDs []string, excludeTxIDs []string) (map[string][]models.Transactions, error) {
	excludeSet := make(map[string]bool, len(excludeTxIDs))
	for _, id := range excludeTxIDs {
		excludeSet[id] = true
	}

	result := make(map[string][]models.Transactions)
	for _, tokenID := range tokenIDs {
		txs, err := c.w.GetTransactionsByTokenID(tokenID)
		if err != nil {
			c.log.Warn("GetSyncTransactionChainData: failed to fetch chain", "tokenID", tokenID, "err", err)
			continue
		}
		if len(excludeSet) > 0 {
			filtered := make([]models.Transactions, 0, len(txs))
			for _, tx := range txs {
				if !excludeSet[tx.ID] {
					filtered = append(filtered, tx)
				}
			}
			txs = filtered
		}
		result[tokenID] = txs
	}

	return result, nil
}
