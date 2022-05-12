package ipfsport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/EnsurityTechnologies/logger"
	ipfsnode "github.com/ipfs/go-ipfs-api"
)

type Client struct {
	ensweb.Client
	cfg  *Config
	ipfs *ipfsnode.Shell
	log  logger.Logger
}

func NewClient(cfg *Config, log logger.Logger, ipfs *ipfsnode.Shell) (*Client, error) {
	c := &Client{
		cfg:  cfg,
		log:  log,
		ipfs: ipfs,
	}
	scfg := &srvcfg.Config{
		ServerAddress: "localhost",
		ServerPort:    fmt.Sprintf("%d", cfg.Port),
	}
	var err error
	c.Client, err = ensweb.NewClient(scfg, log)
	if err != nil {
		c.log.Error("failed to create ensweb clent", "err", err)
		return nil, err
	}
	err = c.forwardIPFSPort()
	if err != nil {
		c.log.Error("failed to forward ipfs port", "err", err)
		return nil, err
	}
	return c, nil

}

func (c *Client) forwardIPFSPort() error {
	proto := "/x/" + c.cfg.AppName + "/1.0"
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", c.cfg.Port)
	peer := "/p2p/" + c.cfg.PeerID
	resp, err := c.ipfs.Request("p2p/forward", proto, addr, peer).Send(context.Background())
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (c *Client) SendJSONRequest(path string, method string, req interface{}, resp interface{}, timeout ...time.Duration) error {
	httpReq, err := c.JSONRequest(method, path, req)
	if err != nil {
		return err
	}
	httpResp, err := c.Do(httpReq, timeout...)
	if err != nil {
		return err
	}
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with status code %d", httpResp.StatusCode)
	}
	err = jsonutil.DecodeJSONFromReader(httpResp.Body, resp)
	if err != nil {
		return fmt.Errorf("invalid response")
	}
	return nil
}

func (c *Client) Close() {
	addr := "/ip4/127.0.0.1/tcp/" + fmt.Sprintf("%d", c.cfg.Port)
	req := c.ipfs.Request("p2p/close")
	resp, err := req.Option("listen-address", addr).Send(context.Background())
	if err != nil {
		c.log.Error("failed to close ipfs port")
		return
	}
	if resp.Error != nil {
		c.log.Error("failed to close ipfs port")
	}
}
