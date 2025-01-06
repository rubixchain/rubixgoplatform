package core

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	ExplorerBasePath           string = "/api/v2/services/app/Rubix/"
	ExplorerCreateDIDAPI       string = "CreateOrUpdateRubixUser"
	ExplorerTransactionAPI     string = "CreateOrUpdateRubixTransaction"
	ExplorerCreateDataTransAPI string = "create-datatokens"
	ExplorerMapDIDAPI          string = "map-did"
	ExplorerURLTable           string = "ExplorerURLTable"
)

type ExplorerClient struct {
	ensweb.Client
	log logger.Logger
	es  storage.Storage
}

type ExplorerDID struct {
	PeerID    string `json:"peerid"`
	DID       string `json:"user_did"`
	IPAddress string `json:"ipaddress"`
	Balance   int    `json:"balance"`
}

type ExplorerMapDID struct {
	OldDID string `json:"old_did"`
	NewDID string `json:"new_did"`
	PeerID string `json:"peer_id"`
}

type ExplorerTrans struct {
	TID         string   `json:"transaction_id"`
	SenderDID   string   `json:"sender_did"`
	ReceiverDID string   `json:"receiver_did"`
	TokenTime   float64  `json:"token_time"`
	TokenIDs    []string `json:"token_id"`
	Amount      float64  `json:"amount"`
	TrasnType   int      `json:"transaction_type"`
	QuorumList  []string `json:"quorum_list"`
	DeployerDID string   `json:"deployer_did"`
	ExecutorDID string   `json:"executor_did"`
	//BlockHash   string   `json:"block_hash"`
}

type ExplorerDataTrans struct {
	TID          string                        `json:"transaction_id"`
	CommitterDID string                        `json:"commiter"`
	SenderDID    string                        `json:"sender"`
	ReceiverDID  string                        `json:"receiver"`
	TokenTime    float64                       `json:"token_time"`
	DataTokens   map[string]string             `json:"datatokens"`
	Amount       float64                       `json:"amount"`
	TrasnType    int                           `json:"transaction_type"`
	QuorumList   map[string]map[string]float64 `json:"quorum_list"`
}

type ExplorerResponse struct {
	Message string `json:"Message"`
	Status  bool   `json:"Status"`
}

type ExplorerURL struct {
	URL      string `gorm:"column:url;primaryKey" json:"ExplorerURL"`
	Port     int    `gorm:"column:port" json:"Explorerport"`
	Protocol string `gorm:"column:protocol" json:"explorer_protocol"`
}

func (c *Core) InitRubixExplorer() error {

	err := c.s.Init(ExplorerURLTable, &ExplorerURL{}, true)
	if err != nil {
		c.log.Error("Failed to initialise storage ExplorerURL ", "err", err)
		return err
	}

	url := "deamon-explorer.azurewebsites.net"
	if c.testNet {
		url = "rubix-deamon-api.ensurity.com"
	}

	err = c.s.Read(ExplorerURLTable, &ExplorerURL{}, "url=?", url)
	if err != nil {
		err = c.s.Write(ExplorerURLTable, &ExplorerURL{URL: url, Port: 443, Protocol: "https"})
	}

	if err != nil {
		return err
	}

	cl, err := ensweb.NewClient(&config.Config{ServerAddress: url, ServerPort: "443", Production: "true"}, c.log)
	if err != nil {
		return err
	}
	c.ec = &ExplorerClient{
		Client: cl,
		log:    c.log.Named("explorerclient"),
		es:     c.s,
	}
	return nil
}

func (ec *ExplorerClient) SendExploerJSONRequest(method string, path string, input interface{}, output interface{}) error {

	var urls []string
	urls, err := ec.GetAllExplorer()
	if err != nil {
		return err
	}

	for _, url := range urls {
		req, err := ec.JSONRequestForExplorer(method, ExplorerBasePath+path, input, url)
		if err != nil {
			ec.log.Error("Request could not be sent to : "+url, "err", err)
			continue
		}
		resp, err := ec.Do(req)
		if err != nil {
			ec.log.Error("Failed to get response from explorer : "+url, "err", err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			str := fmt.Sprintf("Http Request failed with status %d for "+url, resp.StatusCode)
			ec.log.Error(str)
			continue
		}
		if output == nil {
			continue
		}
		err = jsonutil.DecodeJSONFromReader(resp.Body, output)
		if err != nil {
			ec.log.Error("Invalid response from the node", "err", err)
			continue
		}
	}
	return nil
}

func (ec *ExplorerClient) ExplorerCreateDID(peerID string, did string) error {
	ed := ExplorerDID{
		PeerID: peerID,
		DID:    did,
	}
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerCreateDIDAPI, &ed, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerMapDID(oldDid string, newDID string, peerID string) error {
	ed := ExplorerMapDID{
		OldDID: oldDid,
		NewDID: newDID,
		PeerID: peerID,
	}
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerMapDIDAPI, &ed, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerTransaction(et *ExplorerTrans) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerDataTransaction(et *ExplorerDataTrans) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerCreateDataTransAPI, et, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer with data transaction", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (c *Core) AddExplorer(links []string) error {

	var eurl []ExplorerURL

	for _, url := range links {
		var protocol string
		if strings.HasPrefix(url, "https") {
			protocol = "https"
			url = strings.TrimPrefix(url, "https://")
		} else if strings.HasPrefix(url, "http") {
			protocol = "http"
			url = strings.TrimPrefix(url, "http://")
		} else {
			protocol = "https"
		}
		eur := ExplorerURL{
			URL:      url,
			Port:     0,
			Protocol: protocol,
		}
		eurl = append(eurl, eur)
	}

	err := c.s.Write(ExplorerURLTable, eurl)
	if err != nil {
		return err
	}
	return nil
}

func (c *Core) RemoveExplorer(links []string) error {

	for _, url := range links {
		if strings.HasPrefix(url, "https") {
			url = strings.TrimPrefix(url, "https://")
		} else if strings.HasPrefix(url, "http") {
			url = strings.TrimPrefix(url, "http://")
		}
		err := c.s.Delete(ExplorerURLTable, &ExplorerURL{}, "url=?", url)

		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) GetAllExplorer() ([]string, error) {
	var urls []string
	urls, err := c.ec.GetAllExplorer()
	if err != nil {
		return nil, err
	}
	return urls, nil
}

func (ec *ExplorerClient) GetAllExplorer() ([]string, error) {
	var eurl []ExplorerURL
	var urls []string
	err := ec.es.Read(ExplorerURLTable, &eurl, "url!=?", "")
	if err != nil {
		return nil, err
	}
	for _, url := range eurl {
		urls = append(urls, fmt.Sprintf("%s://%s", url.Protocol, url.URL))
	}
	return urls, nil
}
