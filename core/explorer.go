package core

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const (
	ExplorerBasePath              string = "/api/v2/services/app/Rubix/"
	ExplorerTokenCreateAPI        string = "/api/token/create"
	ExplorerTokenCreatePartsAPI   string = "/api/token/part/create"
	ExplorerTokenCreateSCAPI      string = "/api/token/sc/create"
	ExplorerTokenCreateFTAPI      string = "/api/token/ft/create"
	ExplorerTokenCreateNFTAPI     string = "/api/token/nft/create"
	ExplorerCreateUserAPI         string = "/api/user/create"
	ExplorerUpdateUserInfoAPI     string = "/api/user/update-user-info"
	ExplorerUpdateTokenInfoAPI    string = "/api/token/update-token-info"
	ExplorerGetUserKeyAPI         string = "/api/user/get-api-key"
	ExplorerGenerateUserKeyAPI    string = "/api/user/generate-api-key"
	ExplorerExpireUserKeyAPI      string = "/api/user/set-expire-api-key"
	ExplorerRBTTransactionAPI     string = "/api/transactions/rbt"
	ExplorerSCTransactionAPI      string = "/api/transactions/sc"
	ExplorerUpdatePledgeStatusAPI string = "/api/token/update-pledge-status"
	ExplorerCreateDataTransAPI    string = "create-datatokens"
	ExplorerMapDIDAPI             string = "map-did"
	ExplorerURLTable              string = "ExplorerURLTable"
	ExplorerUserDetailsTable      string = "ExplorerUserDetails"
)

type ExplorerClient struct {
	ensweb.Client
	log logger.Logger
	es  storage.Storage
}

type ExplorerDID struct {
	PeerID  string  `json:"peer_id"`
	DID     string  `json:"user_did"`
	Balance float64 `json:"balance"`
	DIDType int     `json:"did_type"`
}

type ExplorerMapDID struct {
	OldDID string `json:"old_did"`
	NewDID string `json:"new_did"`
	PeerID string `json:"peer_id"`
}

// TODO
type TokenDetails struct {
	TokenHash     string  `json:"token_hash"`
	TokenValue    float64 `json:"token_value"`
	CurrentOwner  string
	PreviousOwner string
	Miner         string
	BlockIDs      []string
	PledgeDetails PledgeInfo //from latest block if available
	TokenType     int
	TokenLevel    int
	TokenNumber   int
	TokenStatus   int
}

type Token struct {
	TokenHash  string  `json:"token_hash"`
	TokenValue float64 `json:"token_value"`
}

type UnpledgeToken struct {
	TokenHashes []string `json:"token_hashes"`
}

type ChildToken struct {
	ChildTokenID string  `json:"token_hash"`
	TokenValue   float64 `json:"token_value"`
}

type SCToken struct {
	SCTokenHash   string `json:"token_hash"`
	SCBlockHash   string `json:"block_hash"`
	SCBlockNumber int    `json:"block_number"`
}

type ExplorerCreateToken struct {
	TokenID     string     `json:"token_hash"`
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
	ParentToken    string       `json:"parent_token_hash"`
	ChildTokenList []ChildToken `json:"child_tokens"`
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
	PledgeDetails    map[string][]string `json:"pledge_details"`
	PledgedTokenList []Token             `json:"pledged_token_list"`
}

type ExplorerRBTTrans struct {
	TokenHashes    []string   `json:"token_hash"`
	TransactionID  string     `json:"transaction_id"`
	Network        int        `json:"network"`
	BlockHash      string     `json:"block_hash"` //it will be different for each token
	SenderDID      string     `json:"sender"`
	ReceiverDID    string     `json:"receiver"`
	Amount         float64    `json:"amount"`
	QuorumList     []string   `json:"quorum_list"`
	PledgeInfo     PledgeInfo `json:"pledge_info"`
	TransTokenList []Token    `json:"token_list"`
	Comments       string     `json:"comments"`
}
type ExplorerSCTrans struct {
	SCBlockHash        []SCToken  `json:"sc_block_hash"`
	TransactionID      string     `json:"transaction_id"`
	Network            int        `json:"network"`
	ExecutorDID        string     `json:"executor"`
	DeployerDID        string     `json:"deployer"`
	Creator            string     `json:"creator"`
	PledgeAmount       float64    `json:"pledge_amount"`
	QuorumList         []string   `json:"quorum_list"`
	PledgeInfo         PledgeInfo `json:"pledge_info"`
	CommittedTokenList []Token    `json:"token_list"`
	Comments           string     `json:"comments"`
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
	DID    string `gorm:"column:did;primaryKey" json:"user_did"`
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
	url := "rexplorer.azurewebsites.net"
	err = c.s.Read(ExplorerURLTable, &ExplorerURL{}, "url=?", url)
	if err != nil {
		err = c.s.Write(ExplorerURLTable, &ExplorerURL{URL: url, Port: 443})
	}

	if err != nil {
		return err
	}

	cl, err := ensweb.NewClient(&config.Config{ServerAddress: url, ServerPort: "0", Production: "true"}, c.log)
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
	}
	return nil
}

func (c *Core) ExplorerUserCreate() []string {
	didList := []wallet.DIDType{}
	dids := []string{}

	err := c.s.Read(wallet.DIDStorage, &didList, "did!=?", "")
	if err != nil {
		c.log.Error("Error reading the DID Storage or DID Storage empty")
		return nil
	}
	for _, d := range didList {
		eu := ExplorerUser{}
		err = c.s.Read(ExplorerUserDetailsTable, &eu, "did=?", d.DID)
		if err != nil {
			ed := ExplorerDID{}
			ed.DID = d.DID
			ed.Balance = 0
			ed.PeerID = c.peerID
			ed.DIDType = d.Type
			err = c.ec.ExplorerUserCreate(&ed)
			if err != nil {
				c.log.Error("Error creating user for did %v", d.DID)
			}
		}
		dids = append(dids, d.DID)
	}
	return dids
}

func (ec *ExplorerClient) ExplorerUserCreate(ed *ExplorerDID) error {
	var er ExplorerUserCreateResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerCreateUserAPI, &ed, &er)
	if err != nil {
		return err
	}
	if er.Message != "User created successfully!" {
		ec.log.Error("Failed to create user for %v with error message %v", ed.DID, er.Message)
		return fmt.Errorf("failed to create user")
	}
	ec.AddDIDKey(ed.DID, er.APIKey)
	ec.log.Info(er.Message + " for did " + ed.DID)
	return nil
}

func (c *Core) UpdateUserInfo(dids []string) {
	for _, did := range dids {
		accInfo, err := c.GetAccountInfo(did)
		if err != nil {
			c.log.Error("Failed to get account info for DID %v", did)
			continue
		}
		var er ExplorerResponse
		ed := ExplorerDID{}
		ed.PeerID = c.peerID
		ed.Balance = accInfo.RBTAmount
		ed.DIDType = 4
		err = c.ec.SendExploerJSONRequest("PUT", ExplorerUpdateUserInfoAPI+"/"+did, &ed, &er)
		if err != nil {
			c.log.Error("Failed to update user info, " + err.Error())
		}
		if er.Message != "User balance updated successfully!" {
			c.log.Error("Failed to update user info, ", "msg", er.Message)
		}
		c.log.Info(fmt.Sprintf("%v for did %v", er.Message, did))
	}
}

func (c *Core) UpdateTokenInfo() {
	tokenList := []wallet.Token{}
	// dids := []string{}

	err := c.s.Read(wallet.TokenStorage, &tokenList, "token_id!=?", "")
	if err != nil {
		c.log.Error("Error reading the DID Storage or DID Storage empty")
		return
	}
	tokensToSend := []TokenDetails{}
	count := 0
	l := len(tokenList)
	for i, token := range tokenList {
		if token.Added {
			continue
		}
		td := c.populateTokenDetail(token)
		//populate td
		tokensToSend = append(tokensToSend, td)
		count += 1

		if count != 10 && i != (l-1) {
			continue
		}
		var er ExplorerResponse
		err = c.ec.SendExploerJSONRequest("POST", ExplorerUpdateTokenInfoAPI, &tokensToSend, &er)
		if err != nil {
			c.log.Error("Failed to update user info, " + err.Error())
		}
		if !er.Status {
			c.log.Error("Failed to update user info, ", "msg", er.Message)
			return
		}

		tokensUpdated := strings.Split(er.Message, ",")
		for _, t := range tokensUpdated {
			t1 := wallet.Token{}
			err := c.s.Read(wallet.TokenStorage, &t1, "token_id=?", t)
			if err != nil {
				c.log.Error("Error reading the DID Storage or DID Storage empty")
				return
			}
			t1.Added = true
			err = c.s.Update(wallet.TokenStorage, &t1, "token_id=?", t)
			if err != nil {
				c.log.Error("Error reading the DID Storage or DID Storage empty")
				return
			}
		}
		count = 0
		tokensToSend = []TokenDetails{}
	}
}

func (c *Core) populateTokenDetail(token wallet.Token) TokenDetails {
	td := TokenDetails{}
	td.TokenHash = token.TokenID
	td.TokenValue = token.TokenValue
	td.TokenStatus = token.TokenStatus
	td.CurrentOwner = ""
	td.Miner = ""
	td.PreviousOwner = ""
	td.BlockIDs = []string{}
	td.PledgeDetails = PledgeInfo{}
	td.TokenLevel = -1
	td.TokenNumber = -1
	//Get token type
	typeString := RBTString
	if token.TokenValue < 1.0 {
		typeString = PartString
	}
	td.TokenType = c.TokenType(typeString)
	var blocks [][]byte
	var nextBlockID string
	blockId := ""
	var err error
	//This for loop ensures that we fetch all the blocks in the token chain
	//starting from genesis block to latest block
	for {
		//GetAllTokenBlocks returns next 100 blocks and nextBlockID of the 100th block, starting from the given block Id, in the direction: genesis to latest block
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(token.TokenID, td.TokenType, blockId)
		if err != nil {
			return TokenDetails{}
		}
		//the nextBlockID of the latest block is empty string
		blockId = nextBlockID
		if nextBlockID == "" {
			break
		}
	}

	//Once we have all the blocks, we traverse each block and get the details from each block. If there is a next block after the current block
	//which is of type transaction block, then we get the pledge details from next one. We update the details as we go forward in traversing the
	//token chain blocks 0,1,2,3,4 - 4

	for i := 0; i < len(blocks); i++ {
		b := block.InitBlock(blocks[i], nil)
		var nb *block.Block
		nb = nil
		if i < len(blocks)-1 {
			nb = block.InitBlock(blocks[i+1], nil)
		}

		if b != nil {
			txnType := b.GetTransType()
			switch txnType {
			case block.TokenGeneratedType:
				td.Miner = b.GetOwner()
				bid, _ := b.GetBlockID(token.TokenID)
				td.BlockIDs = append(td.BlockIDs, bid)
				td.TokenLevel, td.TokenNumber = b.GetTokenLevel(token.TokenID)
				if nb != nil && nb.GetTransType() == block.TokenTransferredType {
					continue
				}
				td.CurrentOwner = b.GetOwner()
				fmt.Printf("TD Genesys %+v: ", td)
				//TODO : add pledge details for mined tokens
			case block.TokenTransferredType:
				bid, _ := b.GetBlockID(token.TokenID)
				td.BlockIDs = append(td.BlockIDs, bid)
				if nb != nil && nb.GetTransType() == block.TokenTransferredType {
					continue
				}
				td.CurrentOwner = b.GetOwner()
				td.PreviousOwner = b.GetSenderDID()
				b.GetPledgedTokens()
				fmt.Printf("TD Else %+v: ", td)
				// 	// td.PledgeDetails = b.
			}

		} else {
			c.log.Error("Invalid block")
		}
	}
	return td
}

func (c *Core) GenerateUserAPIKey(dids []string) {
	for _, did := range dids {
		var er ExplorerUserCreateResponse
		eu := ExplorerUser{}
		//Read for api key in table, if empty, then regenerate, because before this, after creating the DID, we are generating the API Key
		err := c.s.Read(ExplorerUserDetailsTable, &eu, "did=?", did)
		if err != nil {
			c.log.Error("Failed to read table")
		}
		if eu.APIKey != "" {
			continue
		}
		err = c.ec.SendExploerJSONRequest("POST", ExplorerGenerateUserKeyAPI, &eu, &er)
		if err != nil {
			c.log.Error(err.Error())
		}
		if er.Message != "API key regenerated successfully!" {
			c.log.Error("Failed to generate API Key for %v. The error msg is %v", did, er.Message)
		}
		c.ec.AddDIDKey(did, er.APIKey)
		c.log.Info(er.Message + " for did " + did)

	}
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
	err := ec.SendExploerJSONRequest("POST", ExplorerTokenCreatePartsAPI, et, &er)
	if err != nil {
		return err
	}
	if er.Message != "Parts Tokens Create successfully!" {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("Parts created successfully for Parent TokenID %v", et.ParentToken))
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
	if er.Message != "RBT transaction created successfully!" {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info("Transaction details for TransactionID %v is stored successfully", et.TransactionID)
	return nil
}

func (ec *ExplorerClient) ExplorerSCTransaction(et *ExplorerSCTrans) error {
	var er ExplorerResponse
	err := ec.SendExploerJSONRequest("POST", ExplorerSCTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if er.Message != "SC transaction created successfully!" {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info("Smart contract transaction details for TransactionID %v is stored successfully", et.TransactionID)
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
	eu := ExplorerUser{}
	if path != ExplorerCreateUserAPI {
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

func (c *Core) ExpireUserAPIKey() {
	fmt.Println("Cleaning started...")
	eus := []ExplorerUser{}
	//Read for api key in table, if not empty, then expire the apiKey and set the field to empty
	err := c.s.Read(ExplorerUserDetailsTable, &eus, "apiKey!=?", "")
	if err != nil {
		c.log.Error("Failed to read table for Expiring the user Key")
	}
	for _, eu := range eus {
		var er ExplorerResponse
		err = c.ec.SendExploerJSONRequest("POST", ExplorerExpireUserKeyAPI, &eu, &er)
		if err != nil {
			c.log.Error(err.Error())
		}
		if er.Message != "API key expired successfully!" {
			c.log.Error("Failed to expire API Key for %v. The error msg is %v", eu.DID, er.Message)
		}
		eu.APIKey = ""
		err = c.s.Update(ExplorerUserDetailsTable, &eu, "did=?", eu.DID)
		if err != nil {
			c.log.Error(err.Error())
		}
		c.log.Info(fmt.Sprintf("%v for DID %v", er.Message, eu.DID))
		fmt.Println("Cleaning completed...")
	}
}

func (c *Core) UpdatePledgeStatus(tokenHashes []string, did string) {
	var er ExplorerResponse
	ut := UnpledgeToken{TokenHashes: tokenHashes}
	err := c.ec.SendExploerJSONRequest("PUT", ExplorerUpdatePledgeStatusAPI+"/"+did, &ut, &er)
	if err != nil {
		c.log.Error(err.Error())
	}
	if er.Message != "Updated Successfully!" {
		c.log.Error("Failed to update explorer", "msg", er.Message)
	}
	c.log.Info(fmt.Sprintf("Pledge status updated successfully for did %v and token hashes %v", did, tokenHashes))
}
