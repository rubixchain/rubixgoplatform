package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
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
	APINotifyUnusedQuorums          string = "/api/notify-unused-quorums"
	APINotifyReceiverToRollback     string = "/api/notify-receiver"
)

const (
	InvalidPasringErr string = "invalid json parsing"
	RubixRootDir      string = "Rubix/"
	DefaultMainNetDB  string = "rubix.db"
	DefaultTestNetDB  string = "rubixtest.db"
	MainNetDir        string = "MainNet"
	TestNetDir        string = "TestNet"
	TestNetDIDDir     string = "TestNetDID/"
	MaxDecimalPlaces  int    = 3
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
	ipfsHealth           *IPFSHealthManager
	ipfsOps              *IPFSOperations
	ipfsRecovery         *IPFSRecoveryManager
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
	as                   storage.Storage
	srv                  *service.Service
	arbitaryMode         bool
	arbitaryAddr         []string
	ec                   *ExplorerClient
	secret               []byte
	quorumCount          int
	noBalanceQuorumCount int
	defaultSetup         bool
	ipfsSem              chan struct{}
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

func NewCore(cfg *config.Config, cfgFile string, encKey string, log logger.Logger, testNet bool, testNetKey string, am bool, defaultSetup bool) (*Core, error) {
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
		cfg:           cfg,
		cfgFile:       cfgFile,
		encKey:        encKey,
		testNet:       testNet,
		testNetKey:    testNetKey,
		quorumRequest: make(map[string]*ConsensusStatus),
		pd:            make(map[string]*PledgeDetails),
		webReq:        make(map[string]*did.DIDChan),
		qc:            make(map[string]did.DIDCrypto),
		pqc:           make(map[string]did.DIDCrypto),
		sd:            make(map[string]*ServiceDetials),
		arbitaryMode:  am,
		secret:        util.GetRandBytes(32),
		defaultSetup:  defaultSetup,
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
	default:
		c.log.Error("Unsupported DB type, please check the configuration", "type", sc.StorageType)
		return nil, fmt.Errorf("unsupported DB type, please check the configuration")
	}

	c.w, err = wallet.InitWallet(c.s, tcDir, c.log)
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
	c.w.SetupWallet(c.ipfs, c)
	c.PingSetup()
	c.CheckQuorumStatusSetup()
	c.GetPeerdidTypeSetup()
	c.peerSetup()
	c.w.AddDIDLastChar()
	c.SetupToken()
	c.QuroumSetup()
	c.PinService()
	//c.RestartIncompleteTokenChainSyncs()
	//c.UnlockFTs()
	// c.selfTransferService()
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

func (c *Core) StopCore() {
	// exp := model.ExploreModel{
	// 	Cmd:    ExpPeerStatusCmd,
	// 	PeerID: c.peerID,
	// 	Status: "Off",
	// }
	// err := c.PublishExplorer(&exp)
	// if err != nil {
	// 	c.log.Error("Failed to publish explorer model", "err", err)
	// 	return
	// }
	time.Sleep(time.Second)

	// Stop IPFS recovery manager
	if c.ipfsRecovery != nil {
		c.ipfsRecovery.Stop()
	}

	// Stop IPFS health manager
	if c.ipfsHealth != nil {
		c.ipfsHealth.Stop()
	}

	c.stopIPFS()
	if c.l != nil {
		c.l.Shutdown()
	}
}

func (c *Core) CreateTempFolder() (string, error) {
	folderName := filepath.Join(c.cfg.DirPath, "temp", uuid.New().String())
	err := os.MkdirAll(folderName, 0755)
	return folderName, err
}

func (c *Core) CreateSCTempFolder() (string, error) {
	folderName := filepath.Join(c.cfg.DirPath, "SmartContract", uuid.New().String())
	err := os.MkdirAll(folderName, 0755)
	return folderName, err
}

func (c *Core) CreateNFTTempFolder() (string, error) {
	folderName := filepath.Join(c.cfg.DirPath, "NFT", uuid.New().String())
	err := os.MkdirAll(folderName, 0755)
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

	nftFolderName := filepath.Join(c.cfg.DirPath, "NFT", nft)
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
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.log.Debug("SetupForienDID: Attempt", "did", didStr, "attempt", attempt, "maxRetries", maxRetries)

		err := c.FetchDID(didStr)
		if err != nil {
			lastErr = err
			c.log.Error("couldn't fetch did", "did", didStr, "attempt", attempt, "err", err)
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to fetch DID %s after %d attempts: %v", didStr, maxRetries, err)
			}
			// Wait a bit before retry
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}

		// Fetching peer's did type
		peerInfo, err := c.GetPeerDIDInfo(didStr)
		if err != nil {
			if peerInfo == nil {
				c.log.Error("failed to get did type of peer did ", didStr, "error", err, "attempt", attempt)
				lastErr = err
				if attempt == maxRetries {
					return nil, fmt.Errorf("failed to get DID type for %s after %d attempts: %v", didStr, maxRetries, err)
				}
				continue
			}
			if strings.Contains(err.Error(), "retry") {
				c.AddPeerDetails(*peerInfo)
			}
		}
		if peerInfo.DIDType == nil || *peerInfo.DIDType == -1 {
			c.log.Error("failed to get did type of peer did ", didStr, "error", err, "attempt", attempt)
			lastErr = fmt.Errorf("invalid DID type for %s", didStr)
			if attempt == maxRetries {
				return nil, lastErr
			}
			continue
		}

		var dc did.DIDCrypto
		switch *peerInfo.DIDType {
		case did.LiteDIDMode:
			dc = did.InitDIDLite(didStr, c.didDir, nil)
		case did.BasicDIDMode:
			dc = did.InitDIDBasic(didStr, c.didDir, nil)
		case did.StandardDIDMode:
			dc = did.InitDIDStandard(didStr, c.didDir, nil)
		case did.WalletDIDMode:
			dc = did.InitDIDWallet(didStr, c.didDir, nil)
		case did.ChildDIDMode:
			dc = did.InitDIDChild(didStr, c.didDir, nil)
		default:
			dc = did.InitDIDBasic(didStr, c.didDir, nil)
		}

		if dc == nil {
			c.log.Error("Failed to initialize DID", "did", didStr, "type", *peerInfo.DIDType, "attempt", attempt)
			lastErr = fmt.Errorf("failed to initialize DID %s with type %d", didStr, *peerInfo.DIDType)
			if attempt == maxRetries {
				return nil, lastErr
			}
			continue
		}

		c.log.Debug("Successfully initialized DID", "did", didStr, "type", *peerInfo.DIDType)
		return dc, nil
	}

	// This should never be reached, but just in case
	return nil, fmt.Errorf("Failed to setup foreign DID for %s after %d attempts: %v", didStr, maxRetries, lastErr)
}

// Initializes the quorum in it's corresponding did mode (basic/ lite)
func (c *Core) SetupForienDIDQuorum(didStr string, selfDID string) (did.DIDCrypto, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.log.Debug("SetupForienDIDQuorum: Attempt", "did", didStr, "attempt", attempt, "maxRetries", maxRetries)

		err := c.FetchDID(didStr)
		if err != nil {
			lastErr = err
			c.log.Error("couldn't fetch did", "did", didStr, "attempt", attempt, "err", err)
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to fetch DID %s after %d attempts: %v", didStr, maxRetries, err)
			}
			// Wait a bit before retry
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}

		// Fetching peer's did type
		peerInfo, err := c.GetPeerDIDInfo(didStr)
		if err != nil {
			if peerInfo == nil {
				c.log.Error("failed to get did type of peer did ", didStr, "error", err, "attempt", attempt)
				lastErr = err
				if attempt == maxRetries {
					return nil, fmt.Errorf("failed to get DID type for %s after %d attempts: %v", didStr, maxRetries, err)
				}
				continue
			}
			if strings.Contains(err.Error(), "retry") {
				c.AddPeerDetails(*peerInfo)
			}
		}
		if *peerInfo.DIDType == -1 {
			c.log.Error("failed to get did type of peer did ", didStr, "error", err, "attempt", attempt)
			lastErr = fmt.Errorf("invalid DID type for %s", didStr)
			if attempt == maxRetries {
				return nil, lastErr
			}
			continue
		}

		switch *peerInfo.DIDType {
		case did.BasicDIDMode:
			quorumBasic := did.InitDIDQuorumc(didStr, c.didDir, "")
			if quorumBasic == nil {
				c.log.Error("Failed to initialize BasicDIDMode quorum", "did", didStr, "attempt", attempt)
				lastErr = fmt.Errorf("failed to initialize BasicDIDMode quorum for %s", didStr)
				if attempt == maxRetries {
					return nil, lastErr
				}
				continue
			}
			c.log.Debug("Successfully initialized BasicDIDMode quorum", "did", didStr)
			return quorumBasic, nil

		case did.LiteDIDMode:
			quorumLite := did.InitDIDQuorumLite(didStr, c.didDir, "")
			c.log.Debug("[SetupForienDIDQuorum] Initialized LiteDIDMode for ", didStr, " result=", quorumLite, "attempt", attempt)
			if quorumLite == nil {
				c.log.Warn("LiteDIDMode quorum init failed, attempting FetchDID from IPFS", "did", didStr, "attempt", attempt)
				if err := c.FetchDID(didStr); err != nil {
					c.log.Error("FetchDID also failed", "did", didStr, "err", err, "attempt", attempt)
					lastErr = fmt.Errorf("failed to initialize LiteDIDMode quorum for DID: %s (FetchDID failed: %v)", didStr, err)
					if attempt == maxRetries {
						return nil, lastErr
					}
					continue
				}
				quorumLite = did.InitDIDQuorumLite(didStr, c.didDir, "")
				if quorumLite == nil {
					c.log.Error("Failed to initialize LiteDIDMode quorum after FetchDID", "did", didStr, "attempt", attempt)
					lastErr = fmt.Errorf("failed to initialize LiteDIDMode quorum for DID: %s after FetchDID", didStr)
					if attempt == maxRetries {
						return nil, lastErr
					}
					continue
				}
			}
			c.log.Debug("Successfully initialized LiteDIDMode quorum", "did", didStr)
			return quorumLite, nil

		default:
			quorumDefault := did.InitDIDQuorumc(didStr, c.didDir, "")
			if quorumDefault == nil {
				c.log.Error("Failed to initialize default quorum", "did", didStr, "attempt", attempt)
				lastErr = fmt.Errorf("failed to initialize default quorum for %s", didStr)
				if attempt == maxRetries {
					return nil, lastErr
				}
				continue
			}
			c.log.Debug("Successfully initialized default quorum", "did", didStr)
			return quorumDefault, nil
		}
	}

	// This should never be reached, but just in case
	return nil, fmt.Errorf("Failed to setup foreign DID quorum for %s after %d attempts: %v", didStr, maxRetries, lastErr)
}

func (c *Core) FetchDID(did string) error {
	didPath := filepath.Join(c.didDir, did)
	c.log.Debug("FetchDID: Starting fetch", "did", did, "targetPath", didPath)

	// Try up to 3 times with folder cleanup
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.log.Debug("FetchDID: Attempt", "did", did, "attempt", attempt, "maxRetries", maxRetries)

		_, err := os.Stat(didPath)
		if err != nil {
			c.log.Debug("FetchDID: Directory does not exist, creating and fetching", "did", did, "path", didPath, "attempt", attempt)
			err = os.MkdirAll(didPath, 0755) // Use proper permissions for cross-platform
			if err != nil {
				c.log.Error("failed to create directory", "err", err, "path", didPath)
				return err
			}
		}

		// Try fetching to the directory - use proper path separator
		fetchPath := didPath + string(os.PathSeparator)
		err = c.ipfs.Get(did, fetchPath)
		if err != nil {
			c.log.Error("failed to fetch DID from IPFS", "err", err, "did", did, "target", fetchPath, "attempt", attempt)
			if attempt == maxRetries {
				return err
			}
			// Clean up and retry - use RemoveAll for cross-platform compatibility
			c.log.Debug("FetchDID: Cleaning up failed directory and retrying", "did", did, "attempt", attempt)
			if removeErr := os.RemoveAll(didPath); removeErr != nil {
				c.log.Warn("Failed to remove directory during cleanup", "path", didPath, "err", removeErr)
			}
			continue
		}
		c.log.Debug("FetchDID: Successfully fetched from IPFS", "did", did, "path", didPath, "attempt", attempt)

		// Check if master DID file exists and fetch it if needed
		masterDIDPath := filepath.Join(didPath, didm.MasterDIDFileName)
		_, e := os.Stat(masterDIDPath)
		if e == nil {
			c.log.Debug("FetchDID: Found master DID file, fetching master", "did", did, "masterPath", masterDIDPath)
			var rb []byte
			rb, err = ioutil.ReadFile(masterDIDPath)
			if err != nil {
				c.log.Error("failed to read master DID file", "err", err, "path", masterDIDPath)
				return err
			}
			return c.FetchDID(string(rb))
		}

		// List all files in the directory for debugging
		if files, err := ioutil.ReadDir(didPath); err == nil {
			c.log.Debug("FetchDID: Files in directory", "did", did, "files", func() []string {
				var fileNames []string
				for _, f := range files {
					fileNames = append(fileNames, f.Name())
				}
				return fileNames
			}())
		} else {
			c.log.Warn("FetchDID: Could not list directory contents", "did", did, "err", err)
		}

		// Validate that the DID directory contains required files
		pubKeyPath := filepath.Join(didPath, didm.PubKeyFileName)
		if _, err := os.Stat(pubKeyPath); err != nil {
			c.log.Error("DID directory missing required public key file", "did", did, "path", pubKeyPath, "err", err, "attempt", attempt)

			// Try alternative paths (in case IPFS created different structure)
			altPaths := []string{
				filepath.Join(didPath, didm.PubKeyFileName),      // Standard location
				filepath.Join(didPath, did, didm.PubKeyFileName), // Nested structure
			}

			foundAtAltPath := false
			for _, altPath := range altPaths {
				if _, altErr := os.Stat(altPath); altErr == nil {
					c.log.Info("Found public key at alternative path", "did", did, "altPath", altPath)
					// Copy to expected location
					if copyErr := copyFile(altPath, pubKeyPath); copyErr == nil {
						c.log.Info("Copied public key to expected location", "did", did, "from", altPath, "to", pubKeyPath)
						foundAtAltPath = true
						break
					} else {
						c.log.Warn("Failed to copy public key", "did", did, "err", copyErr)
					}
				}
			}

			// If we found and copied the public key, check again
			if foundAtAltPath {
				if _, finalErr := os.Stat(pubKeyPath); finalErr == nil {
					c.log.Debug("Successfully fetched and validated DID", "did", did, "path", didPath)
					return nil
				}
			}

			// If this is not the last attempt, clean up and retry
			if attempt < maxRetries {
				c.log.Warn("FetchDID: Public key missing, cleaning up directory and retrying", "did", did, "attempt", attempt)
				if removeErr := os.RemoveAll(didPath); removeErr != nil {
					c.log.Warn("Failed to remove directory during cleanup", "path", didPath, "err", removeErr)
				}
				continue
			}

			// Final attempt failed
			return fmt.Errorf("DID directory missing required public key file for %s after %d attempts (checked paths: %v)", did, maxRetries, append([]string{pubKeyPath}, altPaths...))
		}

		// Success - public key found
		c.log.Debug("Successfully fetched and validated DID", "did", did, "path", didPath)
		return nil
	}

	// This should never be reached, but just in case
	return fmt.Errorf("Failed to fetch DID %s after %d attempts", did, maxRetries)
}

// Helper function to copy files with cross-platform compatibility
func copyFile(src, dst string) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer sourceFile.Close()

	// Get source file info for permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info %s: %v", src, err)
	}

	// Create destination directory if it doesn't exist
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", dstDir, err)
	}

	// Create destination file
	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer destFile.Close()

	// Copy the file content
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content from %s to %s: %v", src, dst, err)
	}

	// Ensure the file is written to disk
	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file %s: %v", dst, err)
	}

	return nil
}

func (c *Core) GetNFTFromIpfs(nftTokenHash string, nftFolderHash string) error {
	dirPath := filepath.Join(c.cfg.DirPath, "NFT", nftTokenHash)
	// Check if the directory exists
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		// If the directory does not exist, create it
		err = os.MkdirAll(dirPath, 0755)
		if err != nil {
			c.log.Error("failed to create directory", "err", err, "path", dirPath)
			return err
		}
	} else if err != nil {
		// Handle other errors while checking directory existence
		c.log.Error("failed to check directory existence", "err", err, "path", dirPath)
		return err
	}
	// Fetch NFT data from IPFS and store in the directory
	err = c.ipfs.Get(nftFolderHash, dirPath+string(os.PathSeparator))
	if err != nil {
		c.log.Error("failed to get NFT from IPFS", "err", err, "nftFolderHash", nftFolderHash, "path", dirPath)
		return err
	}
	c.log.Info("NFT data fetched successfully from ipfs", "nftTokenHash", nftTokenHash, "nftFolderHash", nftFolderHash)
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

// GetIPFSHealthManager returns the IPFS health manager
func (c *Core) GetIPFSHealthManager() interface{} {
	return c.ipfsHealth
}

// func (c *Core)ConvertContractToBlock(contract *contract.ContractType) *block.Block {

// }
