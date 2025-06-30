package core

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/model"
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
	ExplorerTokenCreateNFTAPI     string = "/api/v2/token/nft/create"
	ExplorerTokenCreateSCAPI      string = "/api/v2/token/sc/create"
	ExplorerCreateUserAPI         string = "/api/user/create"
	ExplorerUpdateUserInfoAPI     string = "/api/v2/user/update-user-info"
	ExplorerUpdateTokenInfoAPI    string = "/api/token/update-token-info"
	ExplorerGetUserKeyAPI         string = "/api/user/get-api-key"
	ExplorerGenerateUserKeyAPI    string = "/api/user/generate-api-key"
	ExplorerExpireUserKeyAPI      string = "/api/user/set-expire-api-key"
	ExplorerRBTTransactionAPI     string = "/api/transactions/rbt"
	ExplorerSCTransactionAPI      string = "/api/v2/transactions/sc"
	ExplorerFTTransactionAPI      string = "/api/transactions/ft"
	ExplorerNFTTransactionAPI     string = "/api/transactions/nft"
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
	PeerID    string                    `json:"peer_id"`
	DID       string                    `json:"user_did"`
	Balance   float64                   `json:"balance"`
	DIDType   int                       `json:"did_type"`
	FTDetails []model.FTInfoForExplorer `json:"ft_detials"`
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

// NFT and FT
type AllToken struct {
	TokenHash   string `json:"tokenHash"`
	BlockHash   string `json:"blockHash"`
	BlockNumber int    `json:"blockNumber"`
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

type ExplorerSCDeploy struct {
	SCTokenHash        string     `json:"sc_token_hash"`
	SCTokenValue       float64    `json:"sc_token_value"`
	Network            int        `json:"network"`
	SCBlockNumber      int        `json:"block_num"`
	TransactionID      string     `json:"transaction_id"`
	SCBlockHash        string     `json:"block_hash"`
	DeployerDID        string     `json:"deployer_did"`
	Creator            string     `json:"creator_did"`
	PledgeAmount       float64    `json:"pledge_amount"`
	QuorumList         []string   `json:"quorum_list"`
	PledgeInfo         PledgeInfo `json:"pledge_info"`
	CommittedTokenList []Token    `json:"token_list"`
	Comments           string     `json:"comments"`
}

type ExplorerSCTrans struct {
	SCTokenHash   string     `json:"sc_token_hash"`
	SCBlockHash   string     `json:"block_hash"`
	SCBlockNumber int        `json:"block_number"`
	TransactionID string     `json:"transaction_id"`
	Network       int        `json:"network"`
	ExecutorDID   string     `json:"executor_did"`
	DeployerDID   string     `json:"deployer_did"`
	Creator       string     `json:"creator_did"`
	PledgeAmount  float64    `json:"pledge_amount"`
	QuorumList    []string   `json:"quorum_list"`
	PledgeInfo    PledgeInfo `json:"pledge_info"`
	Comments      string     `json:"comments"`
}

type ExplorerNFTDeploy struct {
	NFT            string     `json:"nft"`
	NFTBlockHash   string     `json:"nft_block_hash"`
	NFTBlockNumber int        `json:"nft_block_number"`
	NFTValue       float64    `json:"nft_value"`
	SCTokenHash    string     `json:"sc_token_hash"`
	TransactionID  string     `json:"transaction_id"`
	Network        int        `json:"network"`
	OwnerDID       string     `json:"owner_did"`
	DeployerDID    string     `json:"deployer_did"`
	PledgeAmount   float64    `json:"pledge_amount"`
	QuorumList     []string   `json:"quorum_list"`
	PledgeInfo     PledgeInfo `json:"pledge_info"`
	Comments       string     `json:"comments"`
}

type ExplorerNFTExecute struct {
	NFT            string     `json:"nft"`
	ExecutorDID    string     `json:"executor_did"`
	ReceiverDID    string     `json:"receiver_did"`
	Network        int        `json:"network"`
	Comments       string     `json:"comments"`
	NFTValue       float64    `json:"nft_value"`
	NFTData        string     `json:"nft_data"`
	SCTokenHash    string     `json:"sc_token_hash"`
	NFTBlockHash   string     `json:"block_hash"`
	NFTBlockNumber int        `json:"block_number"`
	PledgeAmount   float64    `json:"pledge_amount"`
	TransactionID  string     `json:"transaction_id"`
	Amount         float64    `json:"amount"`
	QuorumList     []string   `json:"quorum_list"`
	PledgeInfo     PledgeInfo `json:"pledge_info"`
}

type ExplorerFTTrans struct {
	FTBlockHash     []AllToken `json:"ft_block_hash"`
	CreatorDID      string     `json:"creator_did"`
	SenderDID       string     `json:"sender_did"`
	ReceiverDID     string     `json:"receiver_did"`
	FTName          string     `json:"ft_name"`
	FTSymbol        string     `json:"ft_symbol"`
	FTTransferCount int        `json:"ft_transfer_count"`
	Network         int        `json:"network"`
	Comments        string     `json:"comments"`
	FTTokenList     []string   `json:"ft_token_list"`
	TransactionID   string     `json:"transaction_id"`
	Amount          float64    `json:"amount"`
	QuorumList      []string   `json:"quorum_list"`
	PledgeInfo      PledgeInfo `json:"pledge_info"`
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
	URL      string `gorm:"column:url;primaryKey" json:"ExplorerURL"`
	Port     int    `gorm:"column:port" json:"Explorerport"`
	Protocol string `gorm:"column:protocol" json:"explorer_protocol"`
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
	//This is to add the URL when the node starts for the first time.
	// Define the new and old URLs
	newURL := "rexplorer.azurewebsites.net"
	oldURL := "deamon-explorer.azurewebsites.net"
	if c.testNet {
		newURL = "testnet-core-api.rubixexplorer.com"
		oldURL = "rubix-deamon-api.ensurity.com"
	}

	// Remove old URLs if they exist
	// var oldExplorer ExplorerURL
	err = c.s.Read(ExplorerURLTable, &ExplorerURL{}, "url=?", oldURL)
	if err == nil {
		// Old URL exists, delete it
		err = c.s.Delete(ExplorerURLTable, &ExplorerURL{}, "url=?", oldURL)
		if err != nil {
			c.log.Error("Failed to delete old ExplorerURL", "err", err)
			// return err
		}
	}

	var explorerURL ExplorerURL
	err = c.s.Read(ExplorerURLTable, &explorerURL, "url=?", newURL)
	if err != nil {
		err = c.s.Write(ExplorerURLTable, &ExplorerURL{URL: newURL, Port: 443, Protocol: "https"})
		if err != nil {
			c.log.Error("URL could not be added to DB ", "url", newURL)
			return err
		}
	}
	if explorerURL.Protocol == "" {
		explorerURL.Protocol = "https"
		err = c.s.Update(ExplorerURLTable, &explorerURL, "url=?", newURL)
		if err != nil {
			c.log.Error("Protocol could not be updated for ", "url", newURL)
			return err
		}
	}

	cl, err := ensweb.NewClient(&config.Config{ServerAddress: newURL, ServerPort: "0", Production: "true"}, c.log)
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

func (ec *ExplorerClient) SendExplorerJSONRequest(method string, path string, input interface{}, output interface{}) error {

	var urls []string
	urls, err := ec.GetAllExplorer()
	if err != nil {
		return err
	}

	const maxRetries = 3
	for _, url := range urls {
		apiKeyForHeader := ""
		if url == "https://rexplorer.azurewebsites.net" || url == "https://testnet-core-api.rubixexplorer.com" {
			apiKeyForHeader = ec.getAPIKey(path, input, false)
		}

		attempts := 0

		for attempts < maxRetries {
			req, err := ec.JSONRequestForExplorer(method, path, input, url, apiKeyForHeader)
			if err != nil {
				ec.log.Error("Request could not be sent to : "+url, "err", err)
				break
			}
			resp, err := ec.Do(req)
			if err != nil {
				ec.log.Error("Failed to get response from explorer : "+url, "err", err)
				break
			}
			// defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				bodyString := string(bodyBytes)
				str := fmt.Sprintf("Http Request failed with status %d for %s. Response: %s", resp.StatusCode, url, bodyString)
				if strings.Contains(bodyString, "DuplicateKey") {
					return fmt.Errorf("user already exists, duplicate key error")
				}
				if strings.Contains(bodyString, "Invalid API Key") {
					//Fetch the API Key from explorer.
					//Add or Update the new API Key in the DB
					//Run from req, err := ec.JSONRequestForExplorer(method, path, input, url, apiKeyForHeader) line again.
					//Put a count of 3, after 3 trials, it should stop.
					attempts++
					ec.log.Info("Invalid API Key. Retrying request with new API Key...", "Attempt", attempts)
					didReq := ec.getAPIKey(path, input, true)
					if didReq != "" {
						apiKeyForHeader, err = ec.GetAPIKeyFromExplorer(url, didReq)
						if err != nil {
							break
						}
						ec.AddDIDKey(didReq, apiKeyForHeader)
						// time.Sleep(2 * time.Second)
						// apiKeyForHeader = ec.getAPIKey(path, input, false) // Fetch new key after regeneration
					}
					continue
				}
				ec.log.Error(str)
				break
			}
			if output == nil {
				break
			}
			err = jsonutil.DecodeJSONFromReader(bytes.NewReader(bodyBytes), output)
			if err != nil {
				ec.log.Error("Invalid response from the node", "err", err)
				break
			}
			return nil
		}
	}
	return nil
}

func (ec *ExplorerClient) GetAPIKeyFromExplorer(url string, didReq string) (string, error) {
	var er ExplorerUserCreateResponse
	eu := ExplorerUser{DID: didReq}

	// Create request to fetch API key
	req, err := ec.JSONRequestForExplorer("POST", ExplorerGenerateUserKeyAPI, &eu, url, "")
	if err != nil {
		ec.log.Error(fmt.Sprintf("Failed to create request for DID %v: %v", didReq, err.Error()))
		return "", err
	}

	// Send request
	resp, err := ec.Do(req)
	if err != nil {
		ec.log.Error(fmt.Sprintf("Failed to send request for DID %v: %v", didReq, err.Error()))
		return "", err
	}

	// Read response
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		ec.log.Error(fmt.Sprintf("Failed to generate API Key for %v. Response: %s", didReq, string(bodyBytes)))
		return "", fmt.Errorf("failed to generate API key, response: %s", string(bodyBytes))
	}

	// Decode JSON response
	err = jsonutil.DecodeJSONFromReader(bytes.NewReader(bodyBytes), &er)
	if err != nil {
		ec.log.Error(fmt.Sprintf("Invalid response for DID %v: %v", didReq, err.Error()))
		return "", err
	}

	// Check response message
	if !strings.Contains(er.Message, "successfully") {
		errMsg := fmt.Sprintf("Failed to generate API Key for %v. Error: %v", didReq, er.Message)
		ec.log.Error(errMsg)
		return "", fmt.Errorf(errMsg)
	}

	ec.log.Info(fmt.Sprintf("API key generated successfully for DID %v: %s", didReq, er.APIKey))
	return er.APIKey, nil
}

func (c *Core) ExplorerUserCreate() []string {
	didList := []wallet.DIDType{}
	dids := []string{}

	//Read all DIDs from the DB.
	err := c.s.Read(wallet.DIDStorage, &didList, "did!=?", "")
	if err != nil {
		c.log.Error("Error reading the DID Storage or DID Storage empty")
		return nil
	}

	var overallWG sync.WaitGroup
	var mu sync.Mutex
	batchSize := 10 //Number of DIDs per batch
	// Channel to collect errors
	// errChan := make(chan string, len(didList))

	for i := 0; i < len(didList); i += batchSize {
		end := i + batchSize
		if end > len(didList) {
			end = len(didList) // Handle remaining DIDs in the last batch
		}
		// Increment the overall WaitGroup for each batch
		overallWG.Add(1)
		go func(batch []wallet.DIDType) {
			defer overallWG.Done()

			var batchWG sync.WaitGroup
			startSignal := make(chan struct{}) // Signal for goroutines in the batch

			// Launch goroutines for the batch
			for _, d := range batch {
				batchWG.Add(1)
				go func(d wallet.DIDType) {
					defer batchWG.Done()
					<-startSignal // Wait for the signal to start

					eu := ExplorerUser{}
					err = c.s.Read(ExplorerUserDetailsTable, &eu, "did=?", d.DID)
					if err != nil {
						accInfo, err := c.GetAccountInfo(d.DID)
						if err != nil {
							c.log.Error(fmt.Sprintf("Error getting account info for DID %v: %v", d.DID, err))
							return
						}
						ftInfo, err := c.GetFTInfoByDID(d.DID)
						if err != nil {
							c.log.Error("Failed to get ft info for DID %v", d.DID)
							return
						}
						ftInfoForExplorer := make([]model.FTInfoForExplorer, len(ftInfo))
						for i, item := range ftInfo {
							ftInfoForExplorer[i] = model.FTInfoForExplorer{
								FTName:     item.FTName,
								FTCount:    item.FTCount,
								CreatorDID: item.CreatorDID,
							}
						}
						balance := float64(accInfo.RBTAmount) // Convert to float64 if necessary
						ed := ExplorerDID{
							DID:       d.DID,
							Balance:   balance,
							PeerID:    c.peerID,
							DIDType:   d.Type,
							FTDetails: ftInfoForExplorer,
						}
						err = c.ec.ExplorerUserCreate(&ed)
						if err != nil {
							c.log.Error(fmt.Sprintf("Error creating user for DID %v: %v", d.DID, err))
							return
						}
					}

					// Append to the result slice
					mu.Lock()
					dids = append(dids, d.DID)
					mu.Unlock()
				}(d)
			}
			// Signal all goroutines in the batch to start
			close(startSignal) // Unblock all goroutines waiting on the channel

			// Wait for all goroutines in this batch to complete
			batchWG.Wait()

		}(didList[i:end])

	}
	overallWG.Wait()
	return dids
}

func (ec *ExplorerClient) ExplorerUserCreate(ed *ExplorerDID) error {
	var er ExplorerUserCreateResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerCreateUserAPI, &ed, &er)
	if err != nil {
		fmt.Println("err in ExplorerUserCreate", err)
		return err
	}
	fmt.Println("er in ExplorerUserCreate", er)

	if er.Message != "User created successfully!" && er.Message != "User updated successfully!" {
		ec.log.Error("Failed to create or update user for %v with error message %v", ed.DID, er.Message)
		return fmt.Errorf("failed to create or update user")
	}
	ec.AddDIDKey(ed.DID, er.APIKey)
	ec.log.Info(er.Message + " for did " + ed.DID)
	return nil
}

func (c *Core) UpdateUserInfo(dids []string) {
	for _, did := range dids {
		go func(did string) {
			didList := wallet.DIDType{}

			accInfo, err := c.GetAccountInfo(did)
			if err != nil {
				c.log.Error("Failed to get account info for DID %v", did)
				return
			}

			ftInfo, err := c.GetFTInfoByDID(did)
			if err != nil {
				c.log.Error("Failed to get ft info for DID %v", did)
				return
			}

			ftInfoForExplorer := make([]model.FTInfoForExplorer, len(ftInfo))
			for i, item := range ftInfo {
				ftInfoForExplorer[i] = model.FTInfoForExplorer{
					FTName:     item.FTName,
					FTCount:    item.FTCount,
					CreatorDID: item.CreatorDID,
				}
			}
			_ = c.s.Read(wallet.DIDStorage, &didList, "did=?", did)
			var er ExplorerResponse
			ed := ExplorerDID{
				PeerID:    c.peerID,
				Balance:   accInfo.RBTAmount,
				DIDType:   didList.Type,
				FTDetails: ftInfoForExplorer,
			}

			err = c.ec.SendExplorerJSONRequest("PUT", ExplorerUpdateUserInfoAPI+"/"+did, &ed, &er)
			if err != nil {
				c.log.Error("Failed to send request for user DID, " + did + " Error : " + err.Error())
				return
			}

			if !strings.Contains(er.Message, "updated successfully") {
				c.log.Error("Failed to update user info for ", "DID", did, "msg", er.Message)
			} else {
				c.log.Info(fmt.Sprintf("%v for did %v", er.Message, did))
			}
		}(did)
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
		err = c.ec.SendExplorerJSONRequest("POST", ExplorerUpdateTokenInfoAPI, &tokensToSend, &er)
		if err != nil {
			c.log.Error("Failed to update token info, " + err.Error())
		}
		if !er.Status {
			c.log.Error("Failed to update token info, ", "msg", er.Message)
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

		go func(did string) {
			var er ExplorerUserCreateResponse
			eu := ExplorerUser{}
			//Read for api key in table, if empty, then regenerate, because before this, after creating the DID, we are generating the API Key
			err := c.s.Read(ExplorerUserDetailsTable, &eu, "did=?", did)
			if err != nil {
				c.log.Error(fmt.Sprintf("Failed to read table for DID %v", did))
				return

			}
			if eu.APIKey != "" {
				return
			}
			err = c.ec.SendExplorerJSONRequest("POST", ExplorerGenerateUserKeyAPI, &eu, &er)
			if err != nil {
				c.log.Error(fmt.Sprintf("Failed to send request for DID %v: %v", did, err.Error()))
				return
			}
			if !strings.Contains(er.Message, "successfully") {
				c.log.Error(fmt.Sprintf("Failed to generate API Key for %v. The error msg is %v", did, er.Message))
				return
			}
			c.ec.AddDIDKey(did, er.APIKey)
			c.log.Info("API key regenerated successfully" + " for DID " + did)

		}(did)

	}
}

func (ec *ExplorerClient) ExplorerMapDID(oldDid string, newDID string, peerID string) error {
	ed := ExplorerMapDID{
		OldDID: oldDid,
		NewDID: newDID,
		PeerID: peerID,
	}
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerMapDIDAPI, &ed, &er)
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
	err := ec.SendExplorerJSONRequest("POST", ExplorerTokenCreateAPI, et, &er)
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
	err := ec.SendExplorerJSONRequest("POST", ExplorerTokenCreatePartsAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("Parts created successfully for Parent TokenID %v", et.ParentToken))
	return nil
}

func (ec *ExplorerClient) ExplorerDataTransaction(et *ExplorerDataTrans) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerCreateDataTransAPI, et, &er)
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
	err := ec.SendExplorerJSONRequest("POST", ExplorerRBTTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("Transaction details for TransactionID %v is stored successfully", et.TransactionID))
	return nil
}

func (ec *ExplorerClient) ExplorerSCDeploy(et *ExplorerSCDeploy) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerTokenCreateSCAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("Smart contract deployment details for TransactionID %v is stored successfully with msg=%v", et.TransactionID, er.Message))
	return nil
}

func (ec *ExplorerClient) ExplorerSCTransaction(et *ExplorerSCTrans) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerSCTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("Smart contract transaction details for TransactionID %v is stored successfully with msg=%v", et.TransactionID, er.Message))
	return nil
}

func (ec *ExplorerClient) ExplorerNFTDeploy(et *ExplorerNFTDeploy) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerTokenCreateNFTAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("NFT deployment details for TransactionID %v is stored successfully with message=%v", et.TransactionID, er.Message))
	return nil
}

func (ec *ExplorerClient) ExplorerNFTTransaction(et *ExplorerNFTExecute) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerNFTTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("NFT transaction details for TransactionID %v is stored successfully with message=%v", et.TransactionID, er.Message))
	return nil
}

func (ec *ExplorerClient) ExplorerFTTransaction(et *ExplorerFTTrans) error {
	var er ExplorerResponse
	err := ec.SendExplorerJSONRequest("POST", ExplorerFTTransactionAPI, et, &er)
	if err != nil {
		return err
	}
	if !strings.Contains(er.Message, "successfully") {
		ec.log.Error("Failed to update explorer", "msg", er.Message)
		return fmt.Errorf("failed to update explorer")
	}
	ec.log.Info(fmt.Sprintf("FT transaction details for TransactionID %v is stored successfully with message = %v", et.TransactionID, er.Message))
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

func (c *Core) AddDIDKey(did string, apiKey string) error {
	err := c.ec.AddDIDKey(did, apiKey)
	if err != nil {
		return err
	}
	return nil
}

func (ec *ExplorerClient) AddDIDKey(did string, apiKey string) error {
	var mu sync.Mutex
	eu := ExplorerUser{}
	err := ec.es.Read(ExplorerUserDetailsTable, &eu, "did=?", did)
	mu.Lock()
	if err != nil {
		eu.DID = did
		eu.APIKey = apiKey
		err = ec.es.Write(ExplorerUserDetailsTable, eu)
		if err != nil {
			return fmt.Errorf("failed to insert new API key for DID %s: %w", did, err)
		}
	} else {
		eu.APIKey = apiKey
		err = ec.es.Update(ExplorerUserDetailsTable, &eu, "did=?", did)
		if err != nil {
			return fmt.Errorf("failed to update API key for DID %s: %w", did, err)
		}
	}
	mu.Unlock()

	return nil
}

func (ec *ExplorerClient) getAPIKey(path string, input interface{}, getDID bool) string {
	eu := ExplorerUser{}
	if path != ExplorerCreateUserAPI {
		var did string
		switch v := input.(type) {
		case *ExplorerRBTTrans:
			did = v.SenderDID
		case *ExplorerCreateToken:
			did = v.UserDID
		case *ExplorerCreateTokenParts:
			did = v.UserDID
		case *ExplorerFTTrans:
			did = v.SenderDID
		case *ExplorerSCTrans:
			did = v.ExecutorDID
		case *ExplorerSCDeploy:
			did = v.DeployerDID
		case *ExplorerNFTDeploy:
			did = v.DeployerDID
		case *ExplorerNFTExecute:
			did = v.ExecutorDID
		default:
			return ""
		}
		if getDID {
			return did
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
	c.log.Info("Cleaning started...")
	eus := []ExplorerUser{}
	//Read for api key in table, if not empty, then expire the apiKey and set the field to empty
	err := c.s.Read(ExplorerUserDetailsTable, &eus, "apiKey!=?", "")
	if err != nil {
		fmt.Println("err in ExpireUserAPIKey", err)
		c.log.Error("Failed to read table for Expiring the user Key")
	}
	var wg sync.WaitGroup
	for _, eu := range eus {
		wg.Add(1)
		go func(eu ExplorerUser) {
			defer wg.Done()
			var er ExplorerResponse
			err = c.ec.SendExplorerJSONRequest("POST", ExplorerExpireUserKeyAPI, &eu, &er)
			if err != nil {
				c.log.Error(fmt.Sprintf("Failed to send request for DID %v: %v", eu.DID, err.Error()))
				return
			}
			if er.Message != "API key expired successfully!" {
				c.log.Error(fmt.Sprintf("Failed to expire API Key for %v. The error msg is %v", eu.DID, er.Message))
				return
			}
			eu.APIKey = ""
			err = c.s.Update(ExplorerUserDetailsTable, &eu, "did=?", eu.DID)
			if err != nil {
				c.log.Error("Failed to update database for DID %v: %v", eu.DID, err.Error())
				return
			}
			c.log.Info(fmt.Sprintf("%v for DID %v", er.Message, eu.DID))
		}(eu)
		// Wait for all goroutines to complete
		wg.Wait()
		c.log.Info("Cleaning completed...")
	}
}

func (c *Core) UpdatePledgeStatus(tokenHashes []string, did string) {
	var er ExplorerResponse
	ut := UnpledgeToken{TokenHashes: tokenHashes}
	err := c.ec.SendExplorerJSONRequest("PUT", ExplorerUpdatePledgeStatusAPI+"/"+did, &ut, &er)
	if err != nil {
		c.log.Error(err.Error())
	}
	if er.Message != "Updated Successfully!" {
		c.log.Error("Failed to update explorer", "msg", er.Message)
	}
	c.log.Info(fmt.Sprintf("Pledge status updated successfully for did %v and token hashes %v", did, tokenHashes))
}
