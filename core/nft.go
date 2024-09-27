package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

// type DeployNFTRequest struct {
// 	nft        string
// 	did        string
// 	quorumType int
// }

type NFTReq struct {
	DID         string
	UserID      string
	NFTFileInfo string
	NFTFile     string
	NFTPath     string
}

type NFT struct {
	DID             string
	NftFileInfoHash string
	NFTFileHash     string
}

type FetchNFTRequest struct {
	NFT     string
	NFTPath string
}

func (c *Core) CreateNFTRequest(requestID string, createNFTRequest NFTReq) {
	defer os.RemoveAll(createNFTRequest.NFTPath)
	fmt.Println("The request ID in CreateNFTRequest", requestID)
	//	defer os.RemoveAll(smartContractTokenRequest.SCPath)
	// dc, err := c.SetupDID(reqID, createNFTRequest.DID)
	// if err != nil {
	// 	c.log.Error("Failed to setup DID")
	// }

	createNFTResponse := c.createNFT(requestID, createNFTRequest)
	fmt.Println("CreateNFTResponse", createNFTResponse)
	didChannel := c.GetWebReq(requestID)
	if didChannel == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
	}
	didChannel.OutChan <- createNFTResponse
}

func (c *Core) createNFT(requestID string, createNFTRequest NFTReq) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}
	fmt.Println("The request id in createNFT", requestID)
	fmt.Println("The createNFTRequest which is being send :", createNFTRequest)
	userID := createNFTRequest.UserID
	// if !ok {
	// 	c.log.Error("Failed to create NFT, user ID missing")
	// 	basicResponse.Message = "Failed to create NFT, user ID missing"
	// 	return basicResponse
	// }
	fmt.Println("The userID in createNFT is :", userID)
	nftFile, err := os.Open(createNFTRequest.NFTFile)
	if err != nil {
		c.log.Error("Failed to open the file which should be converted to NFT", "err", err)
		return basicResponse
	}
	defer nftFile.Close()

	// Add the file which needs to be converted to NFT to IPFS
	nftFileHash, err := c.ipfs.Add(nftFile)
	if err != nil {
		c.log.Error("Failed to add nft file to IPFS", "err", err)
		return basicResponse
	}

	nftFileInfo, err := os.Open(createNFTRequest.NFTFileInfo)
	if err != nil {
		c.log.Error("Failed to open the file which should be converted to NFT", "err", err)
		return basicResponse
	}
	defer nftFile.Close()
	defer nftFileInfo.Close()

	// Add the file which needs to be converted to NFT to IPFS
	nftFileInfoHash, err := c.ipfs.Add(nftFileInfo)
	if err != nil {
		c.log.Error("Failed to add nft file to IPFS", "err", err)
		return basicResponse
	}

	// nft := rac.RacType{
	// 	Type:      c.TokenType(NFTString),
	// 	DID:       createNFTRequest.DID,
	// 	CreatorID: createNFTRequest.DID,
	// 	TimeStamp: time.Now().String(),
	// 	NFTInfo: &rac.RacNFTInfo{
	// 		Creator:            createNFTRequest.DID,
	// 		ContentHash:        nftFileHash,
	// 		ContentDescription: nftFileInfoHash, //Content Description to be added
	// 	},
	// }

	nft := NFT{
		DID:             createNFTRequest.DID,
		NFTFileHash:     nftFileHash,
		NftFileInfoHash: nftFileInfoHash,
	}

	if err != nil {
		c.log.Error("Failed to create NFT", "err", err)
		return basicResponse
	}
	// racBlocks, err := rac.CreateRac(&nft)
	// if err != nil {
	// 	c.log.Error("Failed to create rac", "err", err)
	// 	return basicResponse
	// }

	// if len(racBlocks) != 1 {
	// 	c.log.Error("Failed to create RAC NFT block")
	// 	return basicResponse
	// }

	// // Update the signature of the RAC block
	// err = racBlocks[0].UpdateSignature(dc)
	// if err != nil {
	// 	c.log.Error("Failed to update DID signature in RAC NFT Block", "err", err)
	// 	return basicResponse
	// }

	// // smartContractTokenJSON, err := json.MarshalIndent(smartContractToken, "", "  ")
	// // if err != nil {
	// // 	c.log.Error("Failed to marshal SmartContractToken struct", "err", err)
	// // 	return basicResponse
	// // }
	// nftData, err := json.Marshal(racBlocks)
	// if err != nil {
	// 	c.log.Error("Failed to marshal RAC NFT block", "err", err)
	// }
	// nftTokenHash, err := c.ipfs.Add(bytes.NewReader(nftData))
	// if err != nil {
	// 	c.log.Error("Failed to add SmartContractToken to IPFS", "err", err)
	// 	return basicResponse
	// }

	nftJSON, err := json.MarshalIndent(nft, "", "  ")
	if err != nil {
		c.log.Error("Failed to marshal nft struct", "err", err)
		return basicResponse
	}

	nftHash, err := c.ipfs.Add(bytes.NewReader(nftJSON))
	if err != nil {
		c.log.Error("Failed to add nft to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("The nft token hash generated ", nftHash)

	// Set the response status and message
	nftTokenResponse := &SmartContractTokenResponse{
		Message: "NFT Token generated successfully",
		Result:  nftHash,
	}
	fmt.Println("NFTResponse : ", nftTokenResponse)
	_, err = c.RenameNFTFolder(createNFTRequest.NFTPath, nftHash)
	if err != nil {
		c.log.Error("Failed to rename NFT folder", "err", err)
		return basicResponse
	}
	//err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: smartContractTokenHash, Deployer: smartContractTokenRequest.DID, BinaryCodeHash: binaryCodeHash, RawCodeHash: rawCodeHash, SchemaCodeHash: schemaCodeHash, ContractStatus: 6})
	nftTokenDetails := wallet.NFT{
		TokenID:     nftHash,
		DID:         nft.DID,
		TokenStatus: 0,
		TokenValue:  0,
	}
	c.w.CreateNFT(&nftTokenDetails) //To be done : Write the created token details onto the db
	// Set the response values
	// basicResponse.Status = true
	// basicResponse.Message = smartContractTokenResponse.Message
	// basicResponse.Result = smartContractTokenResponse
	basicResponse.Status = true
	basicResponse.Message = nftTokenResponse.Message
	basicResponse.Result = nftTokenResponse.Result
	return basicResponse
}

func (c *Core) DeployNFT(reqID string, deployReq model.DeployNFTRequest) {
	br := c.deployNFT(reqID, deployReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) deployNFT(reqID string, deployReq model.DeployNFTRequest) *model.BasicResponse {
	fmt.Println("DeployNFT function ")
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(deployReq.DID)
	if !ok {
		resp.Message = "Invalid Deployer DID"
		return resp
	}
	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Deployer DID of the NFT deployer, " + err.Error()
		return resp
	}
	//check the NFT from the db
	nft, err := c.w.GetNFT(did, deployReq.NFT, false)
	if err != nil {
		c.log.Error("Failed to retrieve nft details from storage", err)
		resp.Message = err.Error()
		return resp
	}

	fmt.Println("The nft info fetched from the db is : ", nft)
	//Get the RBT details from DB for the associated amount/ if token amount is of PArts create
	// rbtTokensToCommitDetails, err := c.GetTokens(didCryptoLib, did, deployReq.RBTAmount, SmartContractDeployMode)
	// if err != nil {
	// 	c.log.Error("Failed to retrieve Tokens to be committed", "err", err)
	// 	resp.Message = "Failed to retrieve Tokens to be committed , err : " + err.Error()
	// 	return resp
	// }

	// rbtTokensToCommit := make([]string, 0)

	// defer c.w.ReleaseTokens(rbtTokensToCommitDetails)

	// for i := range rbtTokensToCommitDetails {
	// 	c.w.Pin(rbtTokensToCommitDetails[i].TokenID, wallet.OwnerRole, did, "NA", "NA", "NA", float64(0)) //TODO: Ensure whether trnxId should be added ?
	// 	rbtTokensToCommit = append(rbtTokensToCommit, rbtTokensToCommitDetails[i].TokenID)
	// }

	//rbtTokenInfoArray := make([]contract.TokenInfo, 0)
	nftInfoArray := make([]contract.TokenInfo, 0)
	// for i := range rbtTokensToCommitDetails {
	// 	tokenTypeString := "rbt"
	// 	if rbtTokensToCommitDetails[i].TokenValue != 1 {
	// 		tokenTypeString = "part"
	// 	}
	// 	tokenType := c.TokenType(tokenTypeString)
	// 	latestBlk := c.w.GetLatestTokenBlock(rbtTokensToCommitDetails[i].TokenID, tokenType)
	// 	if latestBlk == nil {
	// 		c.log.Error("failed to get latest block, invalid token chain")
	// 		resp.Message = "failed to get latest block, invalid token chain"
	// 		return resp
	// 	}
	// 	blockId, err := latestBlk.GetBlockID(rbtTokensToCommitDetails[i].TokenID)
	// 	if err != nil {
	// 		c.log.Error("failed to get block id", "err", err)
	// 		resp.Message = "failed to get block id, " + err.Error()
	// 		return resp
	// 	}
	// 	tokenInfo := contract.TokenInfo{
	// 		Token:      rbtTokensToCommitDetails[i].TokenID,
	// 		TokenType:  tokenType,
	// 		TokenValue: rbtTokensToCommitDetails[i].TokenValue,
	// 		OwnerDID:   rbtTokensToCommitDetails[i].DID,
	// 		BlockID:    blockId,
	// 	}
	// 	rbtTokenInfoArray = append(rbtTokenInfoArray, tokenInfo)
	// }

	nftInfo := contract.TokenInfo{
		Token:      deployReq.NFT,
		TokenType:  c.TokenType(NFTString),
		TokenValue: 0,
		OwnerDID:   did,
	}
	nftInfoArray = append(nftInfoArray, nftInfo)

	consensusContractDetails := &contract.ContractType{
		Type:       contract.NFTDeployType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  1,
		TransInfo: &contract.TransInfo{
			DeployerDID: did,
			NFT:         deployReq.NFT,
			TransTokens: nftInfoArray,
		},
		ReqID: reqID,
	}
	consensusContract := contract.CreateNewContract(consensusContractDetails)
	if consensusContract == nil {
		c.log.Error("Failed to create Consensus contract while deploying nft")
		resp.Message = "Failed to create Consensus contract while deploying nft"
		return resp
	}
	err = consensusContract.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	consensusContractBlock := consensusContract.GetBlock()
	if consensusContractBlock == nil {
		c.log.Error("failed to create consensus contract block while deploying nft")
		resp.Message = "failed to create consensus contract block while deployingn nft"
		return resp
	}
	conensusRequest := &ConensusRequest{
		ReqID:            uuid.New().String(),
		Type:             deployReq.QuorumType,
		DeployerPeerID:   c.peerID,
		ContractBlock:    consensusContract.GetBlock(),
		NFT:              deployReq.NFT,
		Mode:             NFTDeployMode,
		TransactionEpoch: txEpoch,
	}

	txnDetails, _, err := c.initiateConsensus(conensusRequest, consensusContract, didCryptoLib)

	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	//txnDetails.Amount = deployReq.RBTAmount
	txnDetails.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(txnDetails)
	tokens := make([]string, 0)
	//tokens = append(tokens, deployReq.SmartContractToken)
	explorerTrans := &ExplorerTrans{
		TID:         txnDetails.TransactionID,
		DeployerDID: did,
		//Amount:      deployReq.RBTAmount,
		TrasnType:  conensusRequest.Type,
		TokenIDs:   tokens,
		QuorumList: conensusRequest.QuorumList,
		TokenTime:  float64(dif.Milliseconds()),
		//BlockHash:   txnDetails.BlockID,
	}
	c.ec.ExplorerTransaction(explorerTrans)

	c.log.Info("NFT Deployed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("NFT Deployed successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) publishNewNftEvent(newEvent *model.NFTDeployEvent) error {
	topic := newEvent.NFT
	if c.ps != nil {
		err := c.ps.Publish(topic, newEvent)
		if err != nil {
			c.log.Error("Failed to publish new event", "err", err)
		}
		c.log.Info("New state published on NFT " + topic)
	}
	return nil
}

func (c *Core) ExecuteNFT(reqID string, executeReq *model.ExecuteNFTRequest) {
	br := c.executeNFT(reqID, executeReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) executeNFT(reqID string, executeReq *model.ExecuteNFTRequest) *model.BasicResponse {
	fmt.Println("ExecuteNFt function called")
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	_, did, ok := util.ParseAddress(executeReq.Executor)
	if !ok {
		resp.Message = "Invalid Executor DID"
		return resp
	}
	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Executor DID, " + err.Error()
		return resp
	}
	//check the smartcontract token from the DB base
	_, err = c.w.GetNFT(executeReq.Executor, executeReq.NFT, false)
	if err != nil {
		c.log.Error("Failed to retrieve NFT Token details from storage", err)
		resp.Message = err.Error()
		return resp
	}

	//get the gensys block of the amrt contract token
	tokenType := c.TokenType(NFTString)
	gensysBlock := c.w.GetGenesisTokenBlock(executeReq.NFT, tokenType)
	if gensysBlock == nil {
		c.log.Debug("Gensys block is empty - NFT not synced")
		resp.Message = "Gensys block is empty - NFT not synced"
		return resp
	}
	latestBlock := c.w.GetLatestTokenBlock(executeReq.NFT, tokenType)
	currentOwner := latestBlock.GetOwner()
	fmt.Println("The current owner of the NFT is :", currentOwner)
	fmt.Println("The latest block is ", latestBlock)
	// if currentOwner != executeReq.Executor {
	// 	c.log.Error("NFT not owned by the executor")
	// 	resp.Message = "NFT not owned by the executor"
	// 	return resp
	// }
	currentNFTValue := executeReq.NFTValue
	if err != nil {
		c.log.Error("Failed to retrieve NFT Value , ", err)
		resp.Message = err.Error()
		return resp
	}
	if currentNFTValue == 0 {
		c.log.Error("NFT Value cannot be 0, ")
		resp.Message = "NFT Value cannot be 0, "
		return resp
	}

	nftInfoArray := make([]contract.TokenInfo, 0)
	nftInfo := contract.TokenInfo{
		Token:      executeReq.NFT,
		TokenType:  c.TokenType(NFTString),
		TokenValue: float64(currentNFTValue),
		OwnerDID:   executeReq.Receiver,
	}
	nftInfoArray = append(nftInfoArray, nftInfo)

	//create teh consensuscontract
	consensusContractDetails := &contract.ContractType{
		Type:       contract.NFTExecuteType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  float64(currentNFTValue),
		TransInfo: &contract.TransInfo{
			ExecutorDID: did,
			ReceiverDID: executeReq.Receiver,
			Comment:     executeReq.Comment,
			NFT:         executeReq.NFT,
			TransTokens: nftInfoArray,
			NFTValue:    executeReq.NFTValue,
		},
		ReqID: reqID,
	}

	consensusContract := contract.CreateNewContract(consensusContractDetails)
	if consensusContract == nil {
		c.log.Error("Failed to create Consensus contract")
		resp.Message = "Failed to create Consensus contract"
		return resp
	}
	err = consensusContract.UpdateSignature(didCryptoLib)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}

	consensusContractBlock := consensusContract.GetBlock()
	if consensusContractBlock == nil {
		c.log.Error("failed to create consensus contract block")
		resp.Message = "failed to create consensus contract block"
		return resp
	}
	conensusRequest := &ConensusRequest{
		ReqID:            uuid.New().String(),
		Type:             executeReq.QuorumType,
		ExecuterPeerID:   c.peerID,
		ContractBlock:    consensusContract.GetBlock(),
		NFT:              executeReq.NFT,
		Mode:             NFTExecuteMode,
		TransactionEpoch: txEpoch,
	}

	txnDetails, _, err := c.initiateConsensus(conensusRequest, consensusContract, didCryptoLib)

	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)

	txnDetails.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(txnDetails)
	tokens := make([]string, 0)
	tokens = append(tokens, executeReq.NFT)
	explorerTrans := &ExplorerTrans{
		TID:         txnDetails.TransactionID,
		ExecutorDID: did,
		TrasnType:   conensusRequest.Type,
		TokenIDs:    tokens,
		QuorumList:  conensusRequest.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
		//BlockHash:   txnDetails.BlockID,
	}
	c.ec.ExplorerTransaction(explorerTrans)
	/* newEvent := model.NewContractEvent{
		Contract:          executeReq.SmartContractToken,
		Did:               did,
		Type:              ExecuteType,
		ContractBlockHash: "",
	}

	err = c.publishNewEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish smart contract executed info")
	} */

	c.log.Info("NFT Executed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("NFT Executed successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) SubsribeNFTSetup(requestID string, topic string) error {
	fmt.Println("SubscribeNFTSetup function called")
	reqID = requestID
	c.l.AddRoute(APIPeerStatus, "GET", c.peerStatus)
	err := c.ps.SubscribeTopic(topic, c.NFTCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe NFT", topic)
	}
	c.log.Info("Subscribing NFT " + topic + " is successful")
	return err
}

func (c *Core) NFTCallBack(peerID string, topic string, data []byte) {
	fmt.Println("NFTCallBack called ")
	var newEvent model.NewNFTEvent
	var fetchNFT FetchNFTRequest
	requestID := reqID
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to get nft details", "err", err)
	}
	c.log.Info("Update on nft " + newEvent.NFT)
	if newEvent.Type == 1 {
		fetchNFT.NFT = newEvent.NFT
		fetchNFT.NFTPath, err = c.CreateNFTTempFolder()
		if err != nil {
			c.log.Error("Fetch nft failed, failed to create nft folder", "err", err)
			return
		}
		// oldScFolder is set to path of the smartcontract folder
		oldNFTFolder := c.cfg.DirPath + "NFT/" + fetchNFT.NFT
		var isPathExist bool
		//info is set to FileInfo describing the oldScFolder
		info, err := os.Stat(oldNFTFolder)
		//If directory doesn't exist, isPathExist is set to false
		if os.IsNotExist(err) {
			isPathExist = false
		} else {
			isPathExist = info.IsDir()

		}

		if isPathExist {
			c.log.Debug("removing the existing folder:", oldNFTFolder, "to add the new folder")
			os.RemoveAll(oldNFTFolder)
		}
		fetchNFT.NFTPath, err = c.RenameNFTFolder(fetchNFT.NFTPath, fetchNFT.NFT)
		if err != nil {
			c.log.Error("Fetch NFT failed, failed to create NFT folder", "err", err)
			return
		}
		c.FetchNFT(requestID, &fetchNFT)
		c.log.Info("NFT " + fetchNFT.NFT + " files fetching succesful")
	}
	nft := newEvent.NFT
	nftFolderPath := c.cfg.DirPath + "NFT/" + nft
	if _, err := os.Stat(nftFolderPath); os.IsNotExist(err) {
		fetchNFT.NFT = nft
		fetchNFT.NFTPath, err = c.CreateNFTTempFolder()
		if err != nil {
			c.log.Error("Fetch nft failed, failed to create nft folder", "err", err)
			return
		}
		fetchNFT.NFTPath, err = c.RenameNFTFolder(fetchNFT.NFTPath, nft)
		if err != nil {
			c.log.Error("Fetch NFT failed, failed to create NFT folder", "err", err)
			return
		}

		c.FetchNFT(requestID, &fetchNFT)
		c.log.Info("NFT " + nft + " files fetching successful")
	}
	publisherPeerID := peerID
	did := newEvent.Did
	tokenType := token.NFTTokenType
	address := publisherPeerID + "." + did
	p, err := c.getPeer(address, "")
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return
	}
	err = c.syncTokenChainFrom(p, "", nft, tokenType)
	if err != nil {
		c.log.Error("Failed to sync token chain block", "err", err)
		return
	}
	c.log.Info("Token chain of " + nft + " syncing successful")
	// curlUrl, err := c.w.GetSmartContractTokenUrl(smartContractToken)
	// if err != nil {
	// 	c.log.Error("Failed to get smart contract token URL", "err", err)
	// 	return
	// }
	// payload := map[string]interface{}{
	// 	"smart_contract_hash": newEvent.SmartContractToken,
	// 	"port":                c.cfg.NodePort,
	// }
	// payLoadBytes, err := json.Marshal(payload)
	// if err != nil {
	// 	c.log.Error("Failed to marshal JSON", "err", err)
	// 	return
	// }
	// request, err := http.NewRequest("POST", curlUrl, bytes.NewBuffer(payLoadBytes))
	// if err != nil {
	// 	fmt.Println("Error creating HTTP request for smart contract statefile updationcallback: ", err)
	// 	return
	// }
	// request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	// client := &http.Client{}
	// response, err := client.Do(request)
	// if err != nil {
	// 	fmt.Println("Error sending HTTP request for smart contract statefile updation: ", err)
	// 	return
	// }
	// if response.StatusCode != http.StatusOK {
	// 	c.log.Error("Error getting response from SC", "status", response.Status)
	// 	return
	// }
	// responseBodyBytes, err := io.ReadAll(response.Body)
	// if err != nil {
	// 	fmt.Printf("Error reading response body: %s\n", err)
	// 	return
	// }
	// responseBody := string(responseBodyBytes)
	// var responseData map[string]interface{}
	// if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
	// 	c.log.Error("Error parsing JSON:", err)
	// 	return
	// }
	// message, ok := responseData["message"].(string)
	// if !ok {
	// 	c.log.Error("Error: 'message' field not found or not a string")
	// 	return
	// }
	// c.log.Debug(message)
	// defer response.Body.Close()
}

func (c *Core) FetchNFT(requestID string, fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	fmt.Println("The FetchNFT function called")
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	nftJSON, err := c.ipfs.Cat(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("Failed to get NFT from network", "err", err)
		return basicResponse
	}

	// Read the smart contract token JSON
	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("Failed to read NFT from network", "err", err)
		return basicResponse
	}

	// Close the smart contract token JSON reader
	nftJSON.Close()

	// Parse smart contract token JSON into SmartContractToken struct
	var nft NFT
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
		return basicResponse
	}

	// Fetch and store the binary code file
	nftFileInfo, err := c.ipfs.Cat(nft.NftFileInfoHash)
	fmt.Println("The nftfileinfo", nftFileInfo)
	if err != nil {
		c.log.Error("Failed to fetch nftfileinfo from network", "err", err)
		return basicResponse
	}
	defer nftFileInfo.Close()

	nftFileInfoPath := fetchNFTRequest.NFTPath
	err = os.MkdirAll(nftFileInfoPath, 0755)
	if err != nil {
		c.log.Error("Failed to create nft file info directory", "err", err)
		return basicResponse
	}

	nftFileInfoDestPath := filepath.Join(nftFileInfoPath, "nftFileInfo")

	// Read the content of binaryCodeFile
	nftFileInfoContent, err := io.ReadAll(nftFileInfo)
	if err != nil {
		c.log.Error("Failed to read binary code file", "err", err)
		return basicResponse
	}
	//sourceFileName := filepath.Base(nftFileInfo.Name())
	// Write the content to binaryCodeFileDestPath
	err = os.WriteFile(nftFileInfoDestPath+".json", nftFileInfoContent, 0644)
	if err != nil {
		c.log.Error("Failed to write nft file info ", "err", err)
		return basicResponse
	}

	// Define a map to hold the parsed JSON data
	var nftData map[string]interface{}

	// Unmarshal (parse) the JSON into the map
	err = json.Unmarshal(nftFileInfoContent, &nftData)
	if err != nil {
		c.log.Info("Error parsing nft meta data:", err)
	}

	// Extract the "filename" key value
	//filename := nftData["filename"].(string)
	filename, ok := nftData["filename"].(string)
	if !ok {
		c.log.Info("Key 'filename' not found or not a string")
	}
	c.log.Info("File Name :", filename)
	// Fetch and store the raw code file
	nftFile, err := c.ipfs.Cat(nft.NFTFileHash)
	if err != nil {
		c.log.Error("Failed to fetch nft file from IPFS", "err", err)
		return basicResponse
	}
	defer nftFile.Close()

	nftFilePath := fetchNFTRequest.NFTPath
	err = os.MkdirAll(nftFilePath, 0755)
	if err != nil {
		c.log.Error("Failed to create raw code directory", "err", err)
		return basicResponse
	}

	nftFileDestPath := filepath.Join(nftFilePath, filename)

	// Read the content of rawCodeFile
	nftFileContent, err := io.ReadAll(nftFile)
	if err != nil {
		c.log.Error("Failed to read nft file", "err", err)
		return basicResponse
	}

	// Write the content to rawCodeFileDestPath
	err = os.WriteFile(nftFileDestPath, nftFileContent, 0644)
	if err != nil {
		c.log.Error("Failed to write raw code file", "err", err)
		return basicResponse
	}

	//	err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: fetchSmartContractRequest.SmartContractToken, Deployer: smartContractToken.DID, BinaryCodeHash: smartContractToken.BinaryCodeHash, RawCodeHash: smartContractToken.RawCodeHash, SchemaCodeHash: smartContractToken.SchemaCodeHash, ContractStatus: wallet.TokenIsFetched})
	err = c.w.CreateNFT(&wallet.NFT{TokenID: fetchNFTRequest.NFT, DID: "", TokenStatus: 0, TokenValue: 0})
	if err != nil {
		c.log.Error("Failed to create NFT", "err", err)
		return basicResponse
	}
	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "Successfully fetched NFT"
	basicResponse.Result = &nft

	return basicResponse
}
