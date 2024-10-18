package core

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	ExplorerBasePath            string = "/api/v2/services/app/Rubix/"
	ExplorerTokenCreateAPI      string = "/api/token/create"
	ExplorerTokenCreatePartsAPI string = "/api/token/create/parts"
	ExplorerTokenCreateSCAPI    string = "/api/token/create/sc"
	ExplorerTokenCreateFTAPI    string = "/api/token/create/ft"
	ExplorerTokenCreateNFTAPI   string = "/api/token/create/nft"
	ExplorerCreateDIDAPI        string = "/api/user/create"
	ExplorerRBTTransactionAPI   string = "/api/transactions/rbt"
	ExplorerSCTransactionAPI    string = "/api/transactions/sc"
	ExplorerTransactionAPI      string = "CreateOrUpdateRubixTransaction"
	ExplorerCreateDataTransAPI  string = "create-datatokens"
	ExplorerMapDIDAPI           string = "map-did"
	ExplorerURLTable            string = "ExplorerURLTable"
	ExplorerUserDetailsTable    string = "ExplorerUserDetails"
)

type ExplorerClient struct {
	ensweb.Client
	log logger.Logger
	es  storage.Storage
}

type ExplorerDID struct {
	PeerID    string `json:"peer_id"`
	DID       string `json:"user_did"`
	IPAddress string `json:"ip_addess"`
	Balance   int    `json:"balance"`
	DIDType   int    `json:"did_type"`
}

type ExplorerMapDID struct {
	OldDID string `json:"old_did"`
	NewDID string `json:"new_did"`
	PeerID string `json:"peer_id"`
}

type Token struct {
	TokenID    string  `json:"token_id"`
	TokenValue float64 `json:"token_value"`
}

type ChildToken struct {
	TokenID    string  `json:"child_token_hash"`
	TokenValue float64 `json:"child_token_value"`
}

type ExplorerCreateToken struct {
	TokenID     string     `json:"token_id"`
	TokenValue  float64    `json:"token_value"`
	Network     int        `json:"network"`
	BlockNumber int        `json:"block_num"`
	UserDID     string     `json:"user_did"`
	TokenType   int        `json:"token_type"`
	QuorumList  []string   `json:"quorum_list"`
	PledgeInfo  PledgeInfo `json:"pledge_info"`
}

type ExplorerCreateTokenParts struct {
	UserDID        string       `json:"user_did"`
	TokenType      string       `json:"token_type"`
	ParentToken    string       `json:"parent_token_hash"`
	ChildTokenList []ChildToken `json:"child_token_list"`
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

type PledgeInfo struct {
	PledgeDetails map[string][]string `json:"pledge_details"`
	TokenList     []Token             `json:"pledged_token_list"`
}

type ExplorerRBTTrans struct {
	TokenHashes   []string   `json:"token_hash"`
	TransactionID string     `json:"transaction_id"`
	Network       int        `json:"network"`
	BlockHash     string     `json:"block_hash"` //it will be different for each token
	SenderDID     string     `json:"sender"`
	ReceiverDID   string     `json:"receiver"`
	Amount        float64    `json:"amount"`
	QuorumList    []string   `json:"quorum_list"`
	PledgeInfo    PledgeInfo `json:"pledge_info"`
	TokenList     []Token    `json:"token_list"`
	Comments      string     `json:"comments"`
}
type ExplorerSCTrans struct {
	SCTokenHash   string     `json:"sc_token_hash"`
	TransactionID string     `json:"transaction_id"`
	Network       int        `json:"network"`
	BlockNumber   int        `json:"block_number"`
	BlockHash     string     `json:"block_hash"`
	ExecutorDID   string     `json:"executor"`
	DeployerDID   string     `json:"deployer"`
	Creator       string     `json:"creator"`
	PledgeAmount  float64    `json:"pledge_amount"`
	QuorumList    []string   `json:"quorum_list"`
	PledgeInfo    PledgeInfo `json:"pledge_info"`
	Comments      string     `json:"comments"`
}

type ExplorerResponse struct {
	Message string `json:"Message"`
	Status  bool   `json:"Status"`
}

type ExplorerUserCreateResponse struct {
	Message    string `json:"message"`
	APIKey     string `json:"apiKey"`
	Expiration string `json:"expiration"`
}

type ExplorerURL struct {
	URL  string `gorm:"column:url;primaryKey" json:"ExplorerURL"`
	Port int    `gorm:"column:port" json:"Explorerport"`
}

type ExplorerUser struct {
	DID    string `gorm:"column:did;primaryKey" json:"did"`
	APIKey string `gorm:"column:apiKey" json:"apiKey"`
}

func (c *Core) InitRubixExplorer() error {

	err := c.s.Init(ExplorerURLTable, &ExplorerURL{}, true)
	if err != nil {
		c.log.Error("Failed to initialise storage ExplorerURL ", "err", err)
		return err
	}

	err = c.s.Init(ExplorerUserDetailsTable, &ExplorerUser{}, true)
	if err != nil {
		c.log.Error("Failed to initialise storage ExplorerUserDetails ", "err", err)
		return err
	}

	url := "deamon-explorer.azurewebsites.net"
	if c.testNet {
		url = "rubix-deamon-api.ensurity.com"
	}

	err = c.s.Read(ExplorerURLTable, &ExplorerURL{}, "url=?", url)
	if err != nil {
		err = c.s.Write(ExplorerURLTable, &ExplorerURL{URL: url, Port: 443})
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
		apiKeyForHeader := ""
		if url == "https://rexplorer.azurewebsites.net" {
			apiKeyForHeader = ec.getAPIKey(path, input)
		} else {
			apiKeyForHeader = ""
		}
		req, err := ec.JSONRequestForExplorer(method, path, input, url, apiKeyForHeader)
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
			bodyBytes, _ := io.ReadAll(resp.Body)
			str := fmt.Sprintf("Http Request failed with status %d for %s. Response: %s", resp.StatusCode, url, string(bodyBytes))
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
		ec.log.Info("Details successfully stored in Explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerCreateDID(ed *ExplorerDID) error {
	var er ExplorerUserCreateResponse
	ed.IPAddress = "0.0.0.0"
	err := ec.SendExploerJSONRequest("POST", ExplorerCreateDIDAPI, &ed, &er)
	if err != nil {
		return err
	}
	if er.Message != "User created successfully!" {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.AddDIDKey(ed.DID, er.APIKey)
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

func (ec *ExplorerClient) ExplorerTokenCreate(et *ExplorerCreateToken) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerTokenCreateAPI, et, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerTokenCreateParts(et *ExplorerCreateTokenParts) error {
	var er ExplorerResponse
	fmt.Println("Calling API")
	err := ec.SendExploerJSONRequest("POST", ExplorerTokenCreatePartsAPI, et, &er)
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

func (ec *ExplorerClient) ExplorerRBTTransaction(et *ExplorerRBTTrans) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerRBTTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (ec *ExplorerClient) ExplorerSCTransaction(et *ExplorerSCTrans) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerSCTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !er.Status {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	return nil
}

func (c *Core) AddExplorer(links []string) error {

	var eurl []ExplorerURL

	for _, url := range links {
		if strings.HasPrefix(url, "https") {
			url = strings.TrimPrefix(url, "https://")
		} else if strings.HasPrefix(url, "http") {
			url = strings.TrimPrefix(url, "http://")
		}
		eur := ExplorerURL{
			URL:  url,
			Port: 0,
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
		urls = append(urls, fmt.Sprintf("https://%s", url.URL))
	}
	return urls, nil
}

func (c *Core) AddDIDKey(did string, apiKey string) error {
	err := c.ec.AddDIDKey(did, apiKey)
	if err != nil {
		return err
	}
	return nil
}

func (ec *ExplorerClient) AddDIDKey(did string, apiKey string) error {
	eu := ExplorerUser{}
	err := ec.es.Read(ExplorerUserDetailsTable, &eu, "did=?", did)
	if err != nil {
		eu.DID = did
		eu.APIKey = apiKey
		err = ec.es.Write(ExplorerUserDetailsTable, eu)
		if err != nil {
			return err
		}
	} else {
		eu.APIKey = apiKey
		ec.es.Update(ExplorerUserDetailsTable, &eu, "did=?", did)
	}

	return nil
}

func (ec *ExplorerClient) getAPIKey(path string, input interface{}) string {
	// var apiKey string
	eu := ExplorerUser{}
	if path != ExplorerCreateDIDAPI {
		var did string
		switch v := input.(type) {
		case *ExplorerRBTTrans:
			did = v.SenderDID
		case *ExplorerCreateToken:
			did = v.UserDID
		default:
			return "unsupported input type"
		}
		err := ec.es.Read(ExplorerUserDetailsTable, &eu, "did=?", did) //Include explorer URL? TODO
		if err != nil {
			return ""
		}
		if eu.APIKey == "" {
			return ""
		}
		return eu.APIKey
	}
	return ""
}
