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
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type NFTReq struct {
	DID         string
	UserID      string
	NFTFileInfo string
	NFTFile     string
	NFTPath     string
	Data        string
}

type NFT struct {
	DID             string
	NftFileInfoHash string
	NFTFileHash     string
	UserId          string
	NFTData         string
}

type FetchNFTRequest struct {
	NFT         string
	NFTPath     string
	ReceiverDID string
	NFTValue    float64
}

func (c *Core) CreateNFTRequest(requestID string, createNFTRequest NFTReq) {
	defer os.RemoveAll(createNFTRequest.NFTPath)
	createNFTResponse := c.createNFT(requestID, createNFTRequest)
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

	userID := createNFTRequest.UserID
	c.log.Info("The user id is :", userID)
	nftData := createNFTRequest.Data
	// nftDataHash, err := c.ipfs.Add(bytes.NewBufferString(nftData))
	// if err != nil {
	// 	c.log.Error("failed to add nft data to ipfs", "err", err)
	// 	return basicResponse
	// } // If the data passed should be the hash
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

	nft := NFT{
		DID:             createNFTRequest.DID,
		NFTFileHash:     nftFileHash,
		NftFileInfoHash: nftFileInfoHash,
		UserId:          userID,
		NFTData:         nftData,
	}

	if err != nil {
		c.log.Error("Failed to create NFT", "err", err)
		return basicResponse
	}

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
	_, err = c.RenameNFTFolder(createNFTRequest.NFTPath, nftHash)
	if err != nil {
		c.log.Error("Failed to rename NFT folder", "err", err)
		return basicResponse
	}
	nftTokenDetails := wallet.NFT{
		TokenID:     nftHash,
		DID:         nft.DID,
		TokenStatus: 0,
		TokenValue:  0,
	}
	c.w.CreateNFT(&nftTokenDetails)
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
	nftJSON, err := c.ipfs.Cat(deployReq.NFT)
	if err != nil {
		c.log.Error("Failed to get NFT from network", "err", err)
	}

	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("Failed to read NFT from network", "err", err)
	}
	nftJSON.Close()
	var nftToken NFT
	err = json.Unmarshal(nftJSONBytes, &nftToken)

	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
	}

	c.log.Info("The nft info fetched from the db is : ", nft)

	nftInfoArray := make([]contract.TokenInfo, 0)
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
			NFTData:     nftToken.NFTData,
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

func (c *Core) publishNewNftEvent(newEvent *model.NFTEvent) error {
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

func (c *Core) TransferNFT(reqID string, transferReq *model.TransferNFTRequest) {
	br := c.transferNFT(reqID, transferReq)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) transferNFT(reqID string, transferReq *model.TransferNFTRequest) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	_, did, ok := util.ParseAddress(transferReq.Owner)
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
	_, err = c.w.GetNFT(transferReq.Owner, transferReq.NFT, false)
	if err != nil {
		c.log.Error("Failed to retrieve NFT Token details from storage", err)
		resp.Message = err.Error()
		return resp
	}

	//get the gensys block of the amrt contract token
	tokenType := c.TokenType(NFTString)
	gensysBlock := c.w.GetGenesisTokenBlock(transferReq.NFT, tokenType)
	if gensysBlock == nil {
		c.log.Debug("Gensys block is empty - NFT not synced")
		resp.Message = "Gensys block is empty - NFT not synced"
		return resp
	}
	latestBlock := c.w.GetLatestTokenBlock(transferReq.NFT, tokenType)
	currentOwner := latestBlock.GetOwner()
	c.log.Info("The current owner of the NFT is :", currentOwner)
	if currentOwner != transferReq.Owner {
		c.log.Error("NFT not owned by the executor")
		resp.Message = "NFT not owned by the executor"
		return resp
	}
	currentNFTValue := transferReq.NFTValue
	if err != nil {
		c.log.Error("Failed to retrieve NFT Value , ", err)
		resp.Message = err.Error()
		return resp
	}
	// if currentNFTValue == 0 {
	// 	c.log.Error("NFT Value cannot be 0, ")
	// 	resp.Message = "NFT Value cannot be 0, "
	// 	return resp
	// }

	nftInfoArray := make([]contract.TokenInfo, 0)
	nftInfo := contract.TokenInfo{
		Token:      transferReq.NFT,
		TokenType:  c.TokenType(NFTString),
		TokenValue: float64(currentNFTValue),
		OwnerDID:   transferReq.Receiver,
	}
	nftInfoArray = append(nftInfoArray, nftInfo)

	//create teh consensuscontract
	consensusContractDetails := &contract.ContractType{
		Type:       contract.NFTExecuteType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  float64(currentNFTValue),
		TransInfo: &contract.TransInfo{
			ExecutorDID: did,
			ReceiverDID: transferReq.Receiver,
			Comment:     transferReq.Comment,
			NFT:         transferReq.NFT,
			TransTokens: nftInfoArray,
			NFTValue:    transferReq.NFTValue,
			NFTData:     transferReq.NFTData,
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
		Type:             transferReq.QuorumType,
		ExecuterPeerID:   c.peerID,
		ContractBlock:    consensusContract.GetBlock(),
		NFT:              transferReq.NFT,
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
	tokens = append(tokens, transferReq.NFT)
	explorerTrans := &ExplorerTrans{
		TID:         txnDetails.TransactionID,
		ExecutorDID: did,
		TrasnType:   conensusRequest.Type,
		TokenIDs:    tokens,
		QuorumList:  conensusRequest.QuorumList,
		TokenTime:   float64(dif.Milliseconds()),
		//BlockHash:   txnDetails.BlockID,
	}
	err = c.w.UpdateNFTStatus(transferReq.NFT, 4)
	if err != nil {
		c.log.Error("Failed to update NFT status after transferring", err)
	}
	c.ec.ExplorerTransaction(explorerTrans)

	c.log.Info("NFT Transferred successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("NFT Transferred successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) SubsribeNFTSetup(requestID string, topic string) error {
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
	var newEvent model.NFTEvent
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
		oldNFTFolder := c.cfg.DirPath + "NFT/" + fetchNFT.NFT
		var isPathExist bool
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
		fetchNFT.ReceiverDID = newEvent.ReceiverDid

		c.FetchNFT(requestID, &fetchNFT)
		c.log.Info("NFT " + nft + " files fetching successful")
	}
	publisherPeerID := peerID
	did := newEvent.Did
	tokenType := c.TokenType(NFTString)
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
}

func (c *Core) FetchNFT(requestID string, fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	nftJSON, err := c.ipfs.Cat(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("Failed to get NFT from network", "err", err)
		return basicResponse
	}

	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("Failed to read NFT from network", "err", err)
		return basicResponse
	}
	nftJSON.Close()
	var nft NFT
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
		return basicResponse
	}

	nftFileInfo, err := c.ipfs.Cat(nft.NftFileInfoHash)
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
	nftFileInfoContent, err := io.ReadAll(nftFileInfo)
	if err != nil {
		c.log.Error("Failed to read binary code file", "err", err)
		return basicResponse
	}

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

	err = c.w.CreateNFT(&wallet.NFT{TokenID: fetchNFTRequest.NFT, DID: fetchNFTRequest.ReceiverDID, TokenStatus: 0, TokenValue: fetchNFTRequest.NFTValue})
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
