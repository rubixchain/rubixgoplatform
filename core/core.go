package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/apiconfig"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/logger"
	"github.com/EnsurityTechnologies/uuid"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/pubsub"
	"github.com/rubixchain/rubixgoplatform/core/quorum"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

const (
	APIPingPath          string = "/api/ping"
	APIPeerStatus        string = "/api/peerstatus"
	APICreditStatus      string = "/api/creditstatus"
	APIQuorumConsensus   string = "/api/quorum-conensus"
	APIQuorumCredit      string = "/api/quorum-credit"
	APIReqPledgeToken    string = "/api/req-pledge-token"
	APIUpdatePledgeToken string = "/api/update-pledge-token"
	APISignatureRequest  string = "/api/signature-request"
	APISendReceiverToken string = "/api/send-receiver-token"
	APISyncTokenChain    string = "/api/sync-token-chain"
)

const (
	InvalidPasringErr string = "invalid json parsing"
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
	cfg           *config.Config
	cfgFile       string
	encKey        string
	log           logger.Logger
	peerID        string
	lock          sync.RWMutex
	ipfsLock      sync.RWMutex
	qlock         sync.RWMutex
	rlock         sync.Mutex
	ipfs          *ipfsnode.Shell
	ipfsState     bool
	ipfsChan      chan bool
	alphaQuorum   *quorum.Quorum
	d             *did.DID
	pm            *ipfsport.PeerManager
	l             *ipfsport.Listener
	ps            *pubsub.PubSub
	started       bool
	ipfsApp       string
	testNet       bool
	testNetKey    string
	version       string
	quorumRequest map[string]*ConsensusStatus
	pd            map[string]*PledgeDetials
	webReq        map[string]*did.DIDChan
	w             *wallet.Wallet
	qc            map[string]did.DIDCrypto
	sd            map[string]*ServiceDetials
}

func InitConfig(configFile string, encKey string, node uint16) error {
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		nodePort := NodePort + node
		portOffset := MaxPeerConn * node
		cfg := config.Config{
			NodeAddress: "localhost",
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
				BootStrap: []string{"/ip4/46.166.163.226/tcp/4001/p2p/Qmb9vLM1cNDeMq5i5e8xwWMt7vr4QCAt17RWh2zp1cjRpY"},
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

func NewCore(cfg *config.Config, cfgFile string, encKey string, log logger.Logger, testNet bool, testNetKey string) (*Core, error) {
	update := false
	if cfg.CfgData.MainWalletConfig.StorageType == 0 {
		cfg.CfgData.MainWalletConfig.StorageType = storage.StorageDBType
		cfg.CfgData.MainWalletConfig.DBAddress = cfg.DirPath + "Rubix/wallet.db"
		cfg.CfgData.MainWalletConfig.DBType = "Sqlite3"
		update = true
	}
	if cfg.CfgData.MainWalletConfig.TokenChainDir == "" {
		cfg.CfgData.MainWalletConfig.TokenChainDir = cfg.DirPath + "Rubix/MainNet/"
		update = true
	}
	if cfg.CfgData.TestWalletConfig.StorageType == 0 {
		cfg.CfgData.TestWalletConfig.StorageType = storage.StorageDBType
		cfg.CfgData.TestWalletConfig.DBAddress = cfg.DirPath + "Rubix/wallet.db"
		cfg.CfgData.TestWalletConfig.DBType = "Sqlite3"
		update = true
	}
	if cfg.CfgData.TestWalletConfig.TokenChainDir == "" {
		cfg.CfgData.TestWalletConfig.TokenChainDir = cfg.DirPath + "Rubix/TestNet/"
		update = true
	}

	c := &Core{
		cfg:           cfg,
		cfgFile:       cfgFile,
		encKey:        encKey,
		testNet:       testNet,
		testNetKey:    testNetKey,
		quorumRequest: make(map[string]*ConsensusStatus),
		pd:            make(map[string]*PledgeDetials),
		webReq:        make(map[string]*did.DIDChan),
		qc:            make(map[string]did.DIDCrypto),
		sd:            make(map[string]*ServiceDetials),
	}

	c.log = log.Named("Core")

	c.ipfsChan = make(chan bool)

	if update {
		c.updateConfig()
	}
	if _, err := os.Stat(cfg.DirPath + "Rubix/MainNet"); os.IsNotExist(err) {
		err := os.MkdirAll(cfg.DirPath+"Rubix/MainNet", os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("Failed to create main net directory", "err", err)
			return nil, err
		}
	}
	wcfg := &cfg.CfgData.MainWalletConfig
	if testNet {
		if _, err := os.Stat(cfg.DirPath + "Rubix/TestNet"); os.IsNotExist(err) {
			err := os.MkdirAll(cfg.DirPath+"Rubix/TestNet", os.ModeDir|os.ModePerm)
			if err != nil {
				c.log.Error("Failed to create test net directory", "err", err)
				return nil, err
			}
		}
		wcfg = &cfg.CfgData.TestWalletConfig
	}

	w, err := wallet.InitWallet(wcfg, c.log, c.testNet)
	if err != nil {
		c.log.Error("Failed to setup wallet", "err", err)
		return nil, err
	}
	c.w = w

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
	c.pm = ipfsport.NewPeerManager(c.cfg.CfgData.Ports.ReceiverPort+11, 100, c.ipfs, c.log, c.cfg.CfgData.BootStrap)
	c.d = did.InitDID(c.cfg.DirPath, c.log, c.ipfs)
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
	c.peerSetup()
	c.SetupToken()
	c.QuroumSetup()
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
	exp := model.ExploreModel{
		Cmd:    ExpPeerStatusCmd,
		PeerID: c.peerID,
		Status: "On",
	}
	err = c.PublishExplorer(&exp)
	if err != nil {
		c.log.Error("Failed to publish message to explorer", "err", err)
		return false, "Failed to publish message to explorer"
	}
	dt, err := c.w.GetAllDIDs()
	if err == nil && len(dt) > 0 {
		list := make([]string, 0)
		for _, d := range dt {
			list = append(list, d.DID)
		}
		exp = model.ExploreModel{
			Cmd:     ExpDIDPeerMapCmd,
			PeerID:  c.peerID,
			DIDList: list,
		}
		err = c.PublishExplorer(&exp)
		if err != nil {
			c.log.Error("Failed to publish message to explorer", "err", err)
			return false, "Failed to publish message to explorer"
		}
	}
	return true, "Setup Complete"
}

func (c *Core) StopCore() {
	exp := model.ExploreModel{
		Cmd:    ExpPeerStatusCmd,
		PeerID: c.peerID,
		Status: "Off",
	}
	err := c.PublishExplorer(&exp)
	if err != nil {
		c.log.Error("Failed to publish explorer model", "err", err)
		return
	}
	time.Sleep(time.Second)
	c.stopIPFS()
	if c.alphaQuorum != nil {
		c.alphaQuorum.Stop()
	}
	if c.l != nil {
		c.l.Shutdown()
	}
}

func (c *Core) CreateTempFolder() (string, error) {
	folderName := c.cfg.DirPath + "temp/" + uuid.New().String()
	err := os.MkdirAll(folderName, os.ModeDir|os.ModePerm)
	return folderName, err
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

func (c *Core) CreateDID(didCreate *did.DIDCreate) (string, error) {
	did, err := c.d.CreateDID(didCreate)
	if err != nil {
		return "", err
	}
	dt := wallet.DIDType{
		DID:    did,
		DIDDir: didCreate.Dir,
		Type:   didCreate.Type,
		Config: didCreate.Config,
	}
	err = c.w.CreateDID(&dt)
	if err != nil {
		c.log.Error("Failed to create did in the wallet", "err", err)
		return "", err
	}
	// exp := model.ExploreModel{
	// 	Cmd:     ExpDIDPeerMapCmd,
	// 	DIDList: []string{did},
	// 	PeerID:  c.peerID,
	// 	Message: "DID Created Successfully",
	// }
	// err = c.PublishExplorer(&exp)
	// if err != nil {
	// 	return "", err
	// }
	return did, nil
}

func (c *Core) GetDIDs(dir string) []wallet.DIDType {
	dt, err := c.w.GetDIDs(dir)
	if err != nil {
		return nil
	}
	return dt
}

func (c *Core) IsDIDExist(dir string, did string) bool {
	_, err := c.w.GetDIDDir(dir, did)
	if err != nil {
		return false
	}
	return true
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
		return fmt.Errorf("Request does not exist")
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
	switch dt.Type {
	case did.BasicDIDMode:
		return did.InitDIDBasic(didStr, c.cfg.DirPath+"/Rubix", dc), nil
	case did.StandardDIDMode:
		return did.InitDIDStandard(didStr, c.cfg.DirPath+"/Rubix", dc), nil
	case did.WalletDIDMode:
		return did.InitDIDWallet(didStr, c.cfg.DirPath+"/Rubix", dc), nil
	default:
		return nil, fmt.Errorf("DID Type is not supported")
	}
}

func (c *Core) SetupForienDID(didStr string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}
	return did.InitDIDBasic(didStr, c.cfg.DirPath+"/Rubix", nil), nil
}

func (c *Core) SetupForienDIDQuorum(didStr string) (did.DIDCrypto, error) {
	err := c.FetchDID(didStr)
	if err != nil {
		return nil, err
	}
	return did.InitDIDQuorumc(didStr, c.cfg.DirPath+"/Rubix", ""), nil
}

func (c *Core) FetchDID(did string) error {
	_, err := os.Stat(c.cfg.DirPath + "Rubix/" + did)
	if err != nil {
		err = os.MkdirAll(c.cfg.DirPath+"Rubix/"+did, os.ModeDir|os.ModePerm)
		if err != nil {
			c.log.Error("failed to create directory", "err", err)
			return err
		}
		err = c.ipfs.Get(did, c.cfg.DirPath+"Rubix/"+did+"/")
	}
	return err
}
