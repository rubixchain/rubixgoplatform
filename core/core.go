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
				BootStrap: []string{"/ip4/174.141.238.73/tcp/4001/p2p/QmZbukm4Dbhb2LUwMomBPmaiepLroLGueCPTxN1SEUq15u", "/ip4/103.60.213.76/tcp/4023/p2p/12D3KooWE3fSQSb7aTNjS7CRLytWBxL46MeQd1HpBBUtPGz2HAeA"},
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
	c := &Core{
		cfg:        cfg,
		cfgFile:    cfgFile,
		encKey:     encKey,
		testNet:    testNet,
		testNetKey: testNetKey,
	}

	c.log = log.Named("Core")

	c.ipfsChan = make(chan bool)

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
