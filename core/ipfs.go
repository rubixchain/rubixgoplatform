package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/util"
)

const (
	IPFSConfigFilename string = "config"
	SwarmKeyFilename   string = "swarm.key"
)

type DHTAddr struct {
	Addrs []string `json:"Addrs"`
	ID    string   `json:"ID"`
}

type DHTResponse struct {
	Extra     string    `json:"Extra"`
	ID        string    `json:"ID"`
	Responses []DHTAddr `json:"Responses"`
	Type      int       `json:"Type"`
}

// initIPFS wiill initialize IPFS configuration
func (c *Core) initIPFS(ipfsdir string) error {
	c.ipfsApp = "./ipfs"
	if runtime.GOOS == "windows" {
		c.ipfsApp = "./ipfs.exe"
	}
	if _, err := os.Stat(ipfsdir); errors.Is(err, os.ErrNotExist) {
		c.log.Info("Initializing IPFS")
		cmd := exec.Command(c.ipfsApp, "init")
		err := cmd.Run()
		if err != nil {
			c.log.Error("failed to run command", "err", err)
			return err
		}
		time.Sleep(2 * time.Second)
		ipfsConfigFile := ipfsdir + "/" + IPFSConfigFilename
		configData, err := ioutil.ReadFile(ipfsConfigFile)
		if err != nil {
			c.log.Error("failed to read ipfs config file", "err", err)
			return err
		}
		port := fmt.Sprintf("%d", c.cfg.CfgData.Ports.SwarmPort)
		configData = []byte(strings.Replace(string(configData), "4001", port, -1))
		port = fmt.Sprintf("%d", c.cfg.CfgData.Ports.IPFSPort)
		configData = []byte(strings.Replace(string(configData), "5001", port, -1))
		port = fmt.Sprintf("%d", c.cfg.CfgData.Ports.IPFSAPIPort)
		configData = []byte(strings.Replace(string(configData), "8080", port, -1))
		f, err := os.OpenFile(ipfsConfigFile,
			os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, err = f.WriteString(string(configData))
		if err != nil {
			return err
		}
		f.Close()
		if c.testNet {
			_, err = util.Filecopy(c.testNetKey, ipfsdir+"/"+SwarmKeyFilename)
		} else {
			_, err = util.Filecopy(SwarmKeyFilename, ipfsdir+"/"+SwarmKeyFilename)
		}
		if err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
		c.runIPFS()
		c.ipfs = ipfsnode.NewLocalShell()
		if c.ipfs == nil {
			c.log.Error("failed create ipfs shell")
			return fmt.Errorf("failed create ipfs shell")
		}
		_, err = c.ipfs.BootstrapRmAll()
		if err != nil {
			c.log.Error("unable to remove bootstrap", "err", err)
			return err
		}
		_, err = c.ipfs.BootstrapAdd(c.cfg.CfgData.BootStrap)
		if err != nil {
			c.log.Error("unable to add bootstrap", "err", err)
			return err
		}
		err = c.configIPFS()
		if err != nil {
			c.log.Error("unable to do ipfs configuration", "err", err)
			return err
		}
		time.Sleep(2 * time.Second)
		c.stopIPFS()
		c.log.Info("IPFS Initialized")
		return nil
	} else {
		if c.testNet {
			_, err = util.Filecopy(c.testNetKey, ipfsdir+"/"+SwarmKeyFilename)
			time.Sleep(2 * time.Second)
		} else {
			_, err = util.Filecopy(SwarmKeyFilename, ipfsdir+"/"+SwarmKeyFilename)
		}
		if err != nil {
			c.log.Error("failed to copy the test net key", "err", err)
			return err
		}
	}
	return nil
}

// configIPFS will configure IPFS
func (c *Core) configIPFS() error {

	req := c.ipfs.Request("config", "Experimental.Libp2pStreamMounting", "true")
	resp, err := req.Option("bool", true).Send(context.Background())
	if err != nil {
		return err
	}
	defer resp.Close()
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

// runIPFS will run the IPFS
func (c *Core) runIPFS() {
	cmd := exec.Command(c.ipfsApp, "daemon", "--enable-pubsub-experiment")
	c.SetIPFSState(true)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.log.Error("failed to open command stdout", "err", err)
		panic(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.log.Error("failed to open command stdin", "err", err)
		panic(err)
	}
	c.log.Info("Waiting for IPFS daemon to start")
	err = cmd.Start()
	if err != nil {
		c.log.Error("failed to start command", "err", err)
		panic(err)
	}

	go func() {
		<-c.ipfsChan
		if err := cmd.Process.Kill(); err != nil {
			c.log.Error("failed to kill ipfs daemon", "err", err)
		}
		c.log.Info("IPFS daemon requested to close")
		c.log.Info("IPFS daemon finished")
		c.SetIPFSState(false)
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		m := scanner.Text()
		if m == "Daemon is ready" {
			c.log.Info("IPFS Daemon is ready")
			break
		}
		if strings.Contains(m, "Found outdated fs-repo") {
			c.log.Info("IPFS repo needs update")
			b := make([]byte, 2)
			b[0] = 121
			b[1] = 13
			stdin.Write(b)
		}
		c.log.Info(m)
	}

	//time.Sleep(15 * time.Second)
}

// RunIPFS will run the IPFS daemon
func (c *Core) RunIPFS() error {
	os.Setenv("IPFS_PATH", c.cfg.DirPath+".ipfs")
	os.Setenv("LIBP2P_FORCE_PNET", "1")
	err := c.initIPFS(c.cfg.DirPath + ".ipfs")
	if err != nil {
		c.log.Error("failed to initialize IPFS", "err", err)
		return err
	}

	c.runIPFS()

	c.ipfs = ipfsnode.NewLocalShell()

	if c.ipfs == nil {
		c.log.Error("failed create ipfs shell")
		return fmt.Errorf("failed create ipfs shell")
	}

	idoutput, err := c.ipfs.ID()
	if err != nil {
		c.log.Error("unable to get peer id", "err", err)
		return err
	}
	c.peerID = idoutput.ID
	c.log.Info("Node PeerID : " + idoutput.ID)
	return nil
}

// GetIPFSState will get the IPFS running state
func (c *Core) GetIPFSState() bool {
	c.ipfsLock.RLock()
	defer c.ipfsLock.RUnlock()
	return c.ipfsState
}

// SetIPFSState will set the IPFS running state
func (c *Core) SetIPFSState(state bool) {
	c.ipfsLock.Lock()
	defer c.ipfsLock.Unlock()
	c.ipfsState = state
}

// stopIPFS is called to stop IPFS daemon
func (c *Core) stopIPFS() {
	if !c.GetIPFSState() {
		return
	}
	c.ipfsChan <- true
	for {
		if !c.GetIPFSState() {
			break
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (c *Core) AddBootStrap(peers []string) error {
	c.cfg.CfgData.BootStrap = append(c.cfg.CfgData.BootStrap, peers...)
	err := c.updateConfig()
	if err != nil {
		return err
	}
	_, err = c.ipfs.BootstrapAdd(peers)
	return err
}

func (c *Core) RemoveBootStrap(peers []string) error {
	updated := false
	for _, peer := range peers {
		newitems := []string{}
		update := false
		for _, i := range c.cfg.CfgData.BootStrap {
			if i != peer {
				newitems = append(newitems, i)
			} else {
				update = true
			}
		}
		if update {
			c.cfg.CfgData.BootStrap = newitems
			updated = true
		}
	}
	if updated {
		err := c.updateConfig()
		if err != nil {
			return err
		}
		_, err = c.ipfs.BootstrapRmAll()
		if err != nil {
			return err
		}
		if len(c.cfg.CfgData.BootStrap) != 0 {
			_, err = c.ipfs.BootstrapAdd(c.cfg.CfgData.BootStrap)
		}
		return err
	}
	return nil
}

func (c *Core) RemoveAllBootStrap() error {
	c.cfg.CfgData.BootStrap = make([]string, 0)
	err := c.updateConfig()
	if err != nil {
		return err
	}
	_, err = c.ipfs.BootstrapRmAll()
	if err != nil {
		return err
	}
	return nil
}

func (c *Core) GetAllBootStrap() []string {
	return c.cfg.CfgData.BootStrap
}

func (c *Core) GetDHTddrs(cid string) ([]string, error) {
	cmd := exec.Command(c.ipfsApp, "dht", "findprovs", cid)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.log.Error("failed to open command stdout", "err", err)
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		c.log.Error("failed to start command", "err", err)
		return nil, err
	}
	ids := make([]string, 0)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		m := scanner.Text()
		if strings.Contains(m, "Error") {
			return nil, fmt.Errorf(m)
		}
		if !strings.HasPrefix(m, "Qm") {
			ids = append(ids, m)
		}
	}
	return ids, nil
}

func (c *Core) ipfsRepoGc() {
	cmd := exec.Command(c.ipfsApp, "ipfs", "repo", "gc")
	err := cmd.Start()
	if err != nil {
		c.log.Error("failed to start command", "err", err)
		//return nil, err
	}
}
