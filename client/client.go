package client

import (
	"fmt"
	"net/http"
	"time"

	srvcfg "github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/EnsurityTechnologies/logger"
)

type Client struct {
	ensweb.Client
	cfg *srvcfg.Config
	log logger.Logger
}

func NewClient(cfg *srvcfg.Config, log logger.Logger) (*Client, error) {
	ec, err := ensweb.NewClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to get new client, " + err.Error())
	}
	c := &Client{
		Client: ec,
		cfg:    cfg,
		log:    log,
	}
	return c, nil
}

func (c *Client) basicRequest(method string, path string, model interface{}) (*http.Request, error) {
	r, err := c.JSONRequest(method, path, model)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request, " + err.Error())
	}
	return r, nil
}

func (c *Client) multiFormRequest(method string, path string, field map[string]string, files map[string]string) (*http.Request, error) {
	r, err := c.MultiFormRequest(method, path, field, files)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request, " + err.Error())
	}
	return r, nil
}

func (c *Client) sendJSONRequest(method string, path string, input interface{}, output interface{}, timeout ...time.Duration) error {
	req, err := c.basicRequest(method, path, input)
	if err != nil {
		c.log.Error("Failed to get http request")
		return err
	}
	resp, err := c.Do(req, timeout...)
	if err != nil {
		c.log.Error("Failed to get response from the server, " + err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		str := fmt.Sprintf("Http Request failed with status %d", resp.StatusCode)
		c.log.Error(str)
		return fmt.Errorf(str)
	}
	if output == nil {
		return nil
	}
	err = jsonutil.DecodeJSONFromReader(resp.Body, output)
	if err != nil {
		c.log.Error("Invalid response from the node", "err", err)
		return err
	}
	return nil
}
