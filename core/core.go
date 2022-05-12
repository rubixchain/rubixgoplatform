package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/EnsurityTechnologies/apiconfig"
	"github.com/EnsurityTechnologies/logger"
	"github.com/EnsurityTechnologies/uuid"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/quorum"
)

const (
	APIPingPath string = "/api/ping"
)

const (
	InvalidPasringErr string = "invalid json parsing"
)

const (
	NodePort         uint16 = 14500
	SendPort         uint16 = 15010
	RecvPort         uint16 = 15011
	IPFSPort         uint16 = 5001
	SwarmPort        uint16 = 4001
	IPFSAPIPort      uint16 = 8080
	QuorumPort       uint16 = 15040
	AppPort          uint16 = 15090
	SenderQuorumPort uint16 = 15030
	GossipSenderPort uint16 = 15080
	GossipRecvrPort  uint16 = 15081
	PortOffset       uint16 = 100
)

type Core struct {
	cfg         *config.Config
	log         logger.Logger
	peerID      string
	lock        sync.RWMutex
	ipfsLock    sync.RWMutex
	ipfs        *ipfsnode.Shell
	ipfsState   bool
	ipfsChan    chan bool
	alphaQuorum *quorum.Quorum
	d           *did.DID
	l           *ipfsport.Listener
	started     bool
	ipfsApp     string
}

func InitConfig(configFile string, encKey string, node uint16) error {
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		nodePort := NodePort + node
		portOffset := PortOffset * node
		cfg := config.Config{
			NodeAddress: "localhost",
			NodePort:    fmt.Sprintf("%d", nodePort),
			DirPath:     "./",
			CfgData: config.ConfigData{
				Paths: config.Paths{
					TokensPath:     "Rubix/Wallet/TOKENS/",
					TokenChainPath: "Rubix/Wallet/TOKENCHAINS/",
					PaymentsPath:   "Rubix/PaymentsApp/",
					WalletDataPath: "Rubix/Wallet/WALLET_DATA/",
					DataPath:       "Rubix/DATA/",
					LogPath:        "Rubix/LOGGER/",
				},
				Ports: config.Ports{
					SendPort:           (SendPort + portOffset),
					ReceiverPort:       (RecvPort + portOffset),
					GossipReceiverPort: (GossipRecvrPort + portOffset),
					GossipSenderPort:   (GossipSenderPort + portOffset),
					QuorumPort:         (QuorumPort + portOffset),
					Sender2Q1Port:      (SenderQuorumPort + portOffset),
					Sender2Q2Port:      (SenderQuorumPort + portOffset + 1),
					Sender2Q3Port:      (SenderQuorumPort + portOffset + 2),
					Sender2Q4Port:      (SenderQuorumPort + portOffset + 3),
					Sender2Q5Port:      (SenderQuorumPort + portOffset + 4),
					Sender2Q6Port:      (SenderQuorumPort + portOffset + 5),
					Sender2Q7Port:      (SenderQuorumPort + portOffset + 6),
					IPFSPort:           (IPFSPort + node),
					SwarmPort:          (SwarmPort + node),
					IPFSAPIPort:        (IPFSAPIPort + node),
					AppPort:            (AppPort + portOffset),
				},
				SyncConfig: config.SyncConfig{
					SyncIP:     "http://13.76.134.226:9090",
					ExplorerIP: "https://explorer.rubix.network/api/services/app/Rubix",
					AdvisoryIP: "http://13.76.134.226:9595",
					UserDIDIP:  "127.0.0.1",
				},
				ConsensusData: config.ConsensusData{
					ConsensusStatus: true,
					QuorumCount:     21,
				},
				BootStrap: []string{"/ip4/13.76.134.226/tcp/4001/ipfs/QmYthCYD5WFVm6coBsPRGvknGexpf9icBUpw28t18fBnib",
					"/ip4/183.82.0.114/tcp/4001/p2p/QmcjERi3TqKfLdQp4ViSPMyfGj9oxWKZRAprkppxQc2uMm"},
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

func NewCore(cfg *config.Config, log logger.Logger) (*Core, error) {
	c := &Core{
		cfg: cfg,
	}

	c.log = log.Named("Core")

	c.ipfsChan = make(chan bool)

	return c, nil
}

// SetupCore will setup all core ports
func (c *Core) SetupCore() error {
	var err error
	cfg := &ipfsport.Config{AppName: getPingAppName(c.peerID), Port: c.cfg.CfgData.Ports.ReceiverPort + 10}
	c.l, err = ipfsport.NewListener(cfg, c.log, c.ipfs)
	if err != nil {
		return err
	}
	c.d = did.InitDID(c.cfg, c.log, c.ipfs)
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
	err := c.l.Start()
	if err != nil {
		c.log.Error("failed to start ping port", "err", err)
		return false, "Failed to start ping port"
	}
	return true, "Setup Complete"
}

func (c *Core) StopCore() {
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

func (c *Core) CreateDID(secret string, fileName string) (string, error) {
	return c.d.CreateDID(secret, fileName)
}
