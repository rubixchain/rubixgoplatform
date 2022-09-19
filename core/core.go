package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/adapter"
	"github.com/EnsurityTechnologies/apiconfig"
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
	"github.com/rubixchain/rubixgoplatform/core/util"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

const (
	APIPingPath   string = "/api/ping"
	APIPeerStatus string = "/api/peerstatus"
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
	cfg            *config.Config
	cfgFile        string
	encKey         string
	log            logger.Logger
	peerID         string
	lock           sync.RWMutex
	ipfsLock       sync.RWMutex
	qlock          sync.RWMutex
	ipfs           *ipfsnode.Shell
	ipfsState      bool
	ipfsChan       chan bool
	alphaQuorum    *quorum.Quorum
	d              *did.DID
	pm             *ipfsport.PeerManager
	l              *ipfsport.Listener
	ps             *pubsub.PubSub
	started        bool
	ipfsApp        string
	testNet        bool
	testNetKey     string
	explorerStatus bool
	exploreDB      *adapter.Adapter
	version        string
	quorumRequest  map[string]*ConsensusStatus
	w              *wallet.Wallet
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
		cfg.CfgData.MainWalletConfig.DBAddress = cfg.DirPath + "Rubix/MainNet/wallet.db"
		cfg.CfgData.MainWalletConfig.DBType = "Sqlite3"
		update = true

	}
	if cfg.CfgData.TestWalletConfig.StorageType == 0 {
		cfg.CfgData.TestWalletConfig.StorageType = storage.StorageDBType
		cfg.CfgData.TestWalletConfig.DBAddress = cfg.DirPath + "Rubix/TestNet/wallet.db"
		cfg.CfgData.TestWalletConfig.DBType = "Sqlite3"
		update = true
	}

	c := &Core{
		cfg:           cfg,
		cfgFile:       cfgFile,
		encKey:        encKey,
		testNet:       testNet,
		testNetKey:    testNetKey,
		quorumRequest: make(map[string]*ConsensusStatus),
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

	w, err := wallet.InitWallet(wcfg, c.log)
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
	c.d = did.InitDID(c.cfg, c.log, c.ipfs)
	c.ps, err = pubsub.NewPubSub(c.ipfs, c.log)
	if err != nil {
		return err
	}
	if c.cfg.CfgData.Services != nil {
		ecfg, ok := c.cfg.CfgData.Services[ExploreTopic]
		if ok {
			err = c.initExplorer(ecfg)
			if err != nil {
				c.log.Error("Failed to setup explorer DB", "err", err)
				return err
			}
		}
	}
	c.PingSetup()
	c.PeerStatusSetup()
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
	if len(c.cfg.CfgData.DIDList) > 0 {
		exp = model.ExploreModel{
			Cmd:     ExpDIDPeerMapCmd,
			PeerID:  c.peerID,
			DIDList: c.cfg.CfgData.DIDList,
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
	err = c.updateConfig()
	if err != nil {
		return "", err
	}
	exp := model.ExploreModel{
		Cmd:     ExpDIDPeerMapCmd,
		DIDList: []string{did},
		PeerID:  c.peerID,
		Message: "DID Created Successfully",
	}
	err = c.PublishExplorer(&exp)
	if err != nil {
		return "", err
	}
	return did, nil
}

func (c *Core) GetAllDID() []string {
	str := make([]string, 0)
	for _, did := range c.cfg.CfgData.DIDList {
		str = append(str, util.CreateAddress(c.peerID, did))
	}
	return str
}
