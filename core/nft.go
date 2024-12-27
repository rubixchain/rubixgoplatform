package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type NFTReq struct {
	DID      string
	Metadata string
	Artifact string
	NFTPath  string
}

type NFTIpfsInfo struct {
	DID          string
	ArtifactHash string
}

type FetchNFTRequest struct {
	NFT      string
	NFTPath  string
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
	nftFolderHash, err := c.ipfs.AddDir(createNFTRequest.NFTPath)
	if err != nil {
		c.log.Error("Failed to add nft file to IPFS", "err", err)
		return basicResponse
	}
	nft := NFTIpfsInfo{
		DID:          createNFTRequest.DID,
		ArtifactHash: nftFolderHash,
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

	c.log.Info("The NFT token hash generated ", nftHash)

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

	if err := c.w.CreateNFT(&nftTokenDetails, false); err != nil {
		c.log.Error("Failed to write nft to storage", err)
		return basicResponse
	}

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
	var nftToken NFTIpfsInfo
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
			NFTData:     "",
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

	errNFTDeploy := c.SubscribeNFTSetup("", deployReq.NFT)
	if errNFTDeploy != nil {
		errMsg := fmt.Errorf("unable to subscribe to NFT %v while deployment, err: %v", deployReq.NFT, errNFTDeploy)
		c.log.Error(errMsg.Error())
		resp.Message = errMsg.Error()
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
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}

	_, did, ok := util.ParseAddress(executeReq.Owner)
	if !ok {
		resp.Message = "Invalid Executor DID"
		return resp
	}
	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Executor DID, " + err.Error()
		return resp
	}
	//check the nft token from the DB base
	_, err = c.w.GetNFT(executeReq.Owner, executeReq.NFT, false)
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
	c.log.Info("The current owner of the NFT is :", currentOwner)

	if currentOwner != executeReq.Owner {
		c.log.Error("NFT not owned by the executor")
		resp.Message = "NFT not owned by the executor"
		return resp
	}

	if err != nil {
		c.log.Error("Failed to retrieve NFT Value , ", err)
		resp.Message = err.Error()
		return resp
	}
	var receiver string
	var currentNFTValue float64

	// Empty Receiver indicates Self-Execution. Set the receiver to owner
	// and pledge value is set to current NFT value
	if executeReq.Receiver == "" {
		nftToken, err := c.w.GetNFTToken(executeReq.NFT)
		if err != nil {
			errMsg := fmt.Sprintf("unable to fetch NFT info for NFT ID: %v, err: %v", executeReq.NFT, err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}

		currentNFTValue = nftToken.TokenValue
		receiver = executeReq.Owner
	} else {
		currentNFTValue = executeReq.NFTValue
		receiver = executeReq.Receiver
	}

	nftInfoArray := make([]contract.TokenInfo, 0)
	nftInfo := contract.TokenInfo{
		Token:      executeReq.NFT,
		TokenType:  c.TokenType(NFTString),
		TokenValue: float64(currentNFTValue),
		OwnerDID:   receiver,
	}
	nftInfoArray = append(nftInfoArray, nftInfo)

	//create teh consensuscontract
	consensusContractDetails := &contract.ContractType{
		Type:       contract.NFTExecuteType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  float64(currentNFTValue),
		TransInfo: &contract.TransInfo{
			ExecutorDID: did,
			ReceiverDID: receiver,
			Comment:     executeReq.Comment,
			NFT:         executeReq.NFT,
			TransTokens: nftInfoArray,
			NFTValue:    executeReq.NFTValue,
			NFTData:     executeReq.NFTData,
		},
		ReqID: reqID,
	}

	consensusContract := contract.CreateNewContract(consensusContractDetails)
	if consensusContract == nil {
		c.log.Error("Failed to create Consensus contract")
		resp.Message = "Failed to create Consensus contract"
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
	receiverPeerId := c.w.GetPeerID(executeReq.Receiver)
	local := false
	if receiverPeerId == c.peerID || receiverPeerId == "" {
		local = true
	}

	err = c.w.UpdateNFTStatus(executeReq.NFT, wallet.TokenIsTransferred, local, executeReq.Receiver, executeReq.NFTValue)
	if err != nil {
		c.log.Error("Failed to update NFT status after transferring", err)
	}

	c.ec.ExplorerTransaction(explorerTrans)

	c.log.Info("NFT Executed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("NFT Executed successfully in %v", dif)
	resp.Message = msg
	return resp
}

func (c *Core) SubscribeNFTSetup(requestID string, topic string) error {
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
	requestID := reqID
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to get nft details", "err", err)
		return
	}
	c.log.Info("Recieved Update on nft " + newEvent.NFT)

	nft := newEvent.NFT
	

	// Fetch NFT files
	var fetchNFT FetchNFTRequest
	fetchNFT.NFT = nft

	fetchNFTResponse := c.FetchNFT(requestID, &fetchNFT)
	if !fetchNFTResponse.Status {
		c.log.Error("failed to fetch NFT: ", fetchNFTResponse.Message)
		return
	}

	// Sync Token Chain

	executorDid := newEvent.ExecutorDid
	receiverDid := newEvent.ReceiverDid

	nftTokenType := c.TokenType(NFTString)
	publisherAddress := peerID + "." + executorDid
	publisherPeer, err := c.getPeer(publisherAddress, "")
	if err != nil {
		c.log.Error(fmt.Sprintf("failed to get peer: %v, err: %v", peerID, err))
		return
	}

	err = c.syncTokenChainFrom(publisherPeer, "", nft, nftTokenType)
	if err != nil {
		c.log.Error("Failed to sync token chain block", "err", err)
		return
	}

	// Update NFTTable

	latestNFTTokenBlock := c.w.GetLatestTokenBlock(nft, nftTokenType)
	if latestNFTTokenBlock == nil {
		c.log.Error(fmt.Sprintf("failed to get the latest block for NFT: %v", nft))
		return
	}

	currentOwner := latestNFTTokenBlock.GetOwner()

	// Sanity check
	if receiverDid != "" {
		// Sanity check: In case of transfer NFT, it is always expected that receiver DID
		// will always be same as the onwer (extracted from the latest NFT block)
		if currentOwner != receiverDid {
			c.log.Error("nft callback: reciever DID is not same as the owner of NFT extract from its latest token block")
			return
		}
	} else {
		if currentOwner != executorDid {
			c.log.Error("nft callback: executor DID is not same as the owner of NFT extract from its latest token block")
			return
		}
	}

	var tokenStatus int
	// Add check for receiverDid . In case of self-execution, it will be empty
	if !c.w.IsDIDExist(receiverDid) {
		tokenStatus = wallet.TokenIsTransferred
	} else {
		tokenStatus = wallet.TokenIsFree
	}

	err = c.w.CreateNFT(&wallet.NFT{TokenID: nft, DID: currentOwner, TokenStatus: tokenStatus, TokenValue: newEvent.NFTValue}, c.w.IsNFTExists(nft))
	if err != nil {
		c.log.Error("Failed to create NFT", "err", err)
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

	var nft NFTIpfsInfo
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
		return basicResponse
	}

	if err := c.GetNFTFromIpfs(fetchNFTRequest.NFT, nft.ArtifactHash); err != nil {
		c.log.Error("failed to fetch NFT from IPFS", "err", err)
	}

	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "Successfully fetched NFT"
	basicResponse.Result = &nft

	return basicResponse
}

func (c *Core) GetAllNFT() model.NFTList {
	response := model.NFTList{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	nftList, err := c.w.GetAllNFT()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get NFT list", "err", err)
		c.log.Error(errorMsg)
		response.Message = errorMsg
		return response
	}
	nftDetails := make([]model.NFTInfo, 0)
	for _, nft := range nftList {
		nftDetails = append(nftDetails, model.NFTInfo{NFTId: nft.TokenID, Owner: nft.DID, Value: nft.TokenValue})
	}
	response.NFTs = nftDetails
	response.Status = true
	response.Message = "Got All NFTs"

	return response

}

func (c *Core) GetNFTsByDid(did string) model.NFTList {
	response := model.NFTList{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	nftList, err := c.w.GetNFTsByDid(did)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get NFT list of the did: ", did, "err", err)
		c.log.Error(errorMsg)
		response.Message = errorMsg
		return response
	}
	nftDetails := make([]model.NFTInfo, 0)
	for _, nft := range nftList {
		nftDetails = append(nftDetails, model.NFTInfo{NFTId: nft.TokenID, Owner: nft.DID, Value: nft.TokenValue})
	}
	response.NFTs = nftDetails
	response.Status = true
	response.Message = "Got All NFTs"

	return response

}
