package core

import (
	"encoding/json"
	"errors"
	"fmt"
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
	c.w.SetupWallet(c.ipfs)
	c.PingSetup()
	c.CheckQuorumStatusSetup()
	c.GetPeerdidTypeSetup()
	c.peerSetup()
	c.w.AddDIDLastChar()
	c.SetupToken()
	c.QuroumSetup()
	c.PinService()
	c.RestartIncompleteTokenChainSyncs()
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
	c.stopIPFS()
	if c.l != nil {
		c.l.Shutdown()
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
	_, err := os.Stat(c.didDir + did)
	if err != nil {
		err = os.MkdirAll(c.didDir+did, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("failed to create directory", "err", err)
			return err
		}
		err = c.ipfs.Get(did, c.didDir+did+"/")
		if err == nil {
			_, e := os.Stat(c.didDir + did + "/" + didm.MasterDIDFileName)
			// Fetch the master DID also
			if e == nil {
				var rb []byte
				rb, err = ioutil.ReadFile(c.didDir + did + "/" + didm.MasterDIDFileName)
				if err == nil {
					return c.FetchDID(string(rb))
				}
			}
		}
	}
	return err
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
	err = c.ipfs.Get(nftFolderHash, dirPath)
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
