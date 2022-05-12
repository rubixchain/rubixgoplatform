package core

import (
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
)

func (c *Core) initIPFS(ipfsdir string) error {
	c.ipfsApp = "ipfs"
	if runtime.GOOS == "windows" {
		c.ipfsApp = "ipfs.exe"
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
		ipfsConfigFile := ipfsdir + "/config"
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
	}
	return nil
}

func (c *Core) configIPFS() error {

	req := c.ipfs.Request("config", "Experimental.Libp2pStreamMounting", "true")
	resp, err := req.Option("bool", true).Send(context.Background())

	//resp, err := c.ipfs.Request("config", "Experimental.Libp2pStreamMounting", "true").Send(context.Background())

	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (c *Core) runIPFS() {
	cmd := exec.Command(c.ipfsApp, "daemon")
	//cmd.Env = append(cmd.Env, "IPFS_PATH")

	done := make(chan error, 1)
	c.SetIPFSState(true)

	go func() {
		done <- cmd.Run()
	}()
	go func() {
		select {
		case err := <-done:
			if err != nil {
				c.log.Error("failed to run ipfs daemon", "err", err)
			}
		case <-c.ipfsChan:
			if err := cmd.Process.Kill(); err != nil {
				c.log.Error("failed to kill ipfs daemon", "err", err)
			}
			c.log.Debug("IPFS daemon requested to close")
		}
		c.log.Debug("IPFS daemon finished")
		c.SetIPFSState(false)
	}()
	c.log.Info("Waiting for IPFS daemon to start")
	time.Sleep(15 * time.Second)
}

func (c *Core) RunIPFS() error {
	os.Setenv("IPFS_PATH", c.cfg.DirPath+".ipfs")
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
	c.log.Debug("Got the Peer ID", "PeerdID", idoutput.ID)
	return nil
}

func (c *Core) GetIPFSState() bool {
	c.ipfsLock.RLock()
	defer c.ipfsLock.RUnlock()
	return c.ipfsState
}

func (c *Core) SetIPFSState(state bool) {
	c.ipfsLock.Lock()
	defer c.ipfsLock.Unlock()
	c.ipfsState = state
}

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
