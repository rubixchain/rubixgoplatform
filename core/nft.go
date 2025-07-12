package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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
	NFT     string
	NFTPath string
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

	nftHash, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(nftJSON))
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

	_, err := c.w.GetNFTToken(deployReq.NFT)
	if err == nil {
		c.log.Error(fmt.Sprintf("NFT %v has been already been deployed", deployReq.NFT))
		resp.Message = fmt.Sprintf("NFT %v has already been deployed", deployReq.NFT)
		return resp
	}

	didCryptoLib, err := c.SetupDID(reqID, did)
	if err != nil {
		resp.Message = "Failed to setup Deployer DID of the NFT deployer, " + err.Error()
		return resp
	}

	nftInfoArray := make([]contract.TokenInfo, 0)
	nftInfo := contract.TokenInfo{
		Token:      deployReq.NFT,
		TokenType:  c.TokenType(NFTString),
		TokenValue: deployReq.NFTValue,
		OwnerDID:   did,
	}
	nftInfoArray = append(nftInfoArray, nftInfo)

	consensusContractDetails := &contract.ContractType{
		Type:       contract.NFTDeployType,
		PledgeMode: contract.PeriodicPledgeMode,
		TotalRBTs:  float64(deployReq.NFTValue),
		TransInfo: &contract.TransInfo{
			DeployerDID: did,
			NFT:         deployReq.NFT,
			NFTData:     deployReq.NFTData,
			TransTokens: nftInfoArray,
			NFTValue:    deployReq.NFTValue,
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

	txnDetails, _, pds, err := c.initiateConsensus(conensusRequest, consensusContract, didCryptoLib)

	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}

	nftTokenDetails := wallet.NFT{
		TokenID:     deployReq.NFT,
		DID:         deployReq.DID,
		TokenStatus: wallet.TokenIsFree,
		TokenValue:  floatPrecision(deployReq.NFTValue, MaxDecimalPlaces),
		Metadata:    deployReq.NFTMetadata,
		Filename:    deployReq.NFTFileName,
	}

	if err := c.w.CreateNFT(&nftTokenDetails, false); err != nil {
		c.log.Error("Failed to write nft to storage in NFTTokenStorage", err)
		return resp
	}

	newEvent := model.NFTEvent{
		NFT:         nftTokenDetails.TokenID,
		ExecutorDid: nftTokenDetails.DID,
		NFTMetadata: nftTokenDetails.Metadata,
		NFTFileName: nftTokenDetails.Filename,
		NFTValue:    nftTokenDetails.TokenValue,
	}

	err = c.publishNewNftEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish NFT info")
	}

	et := time.Now()
	dif := et.Sub(st)
	//txnDetails.Amount = deployReq.RBTAmount
	txnDetails.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(txnDetails)

	blockNoPart := strings.Split(txnDetails.BlockID, "-")[0]
	// Convert the string part to an int
	blockNoInt, _ := strconv.Atoi(blockNoPart)
	//Rename : TODO
	eTrans := &ExplorerNFTDeploy{
		NFT:            deployReq.NFT,
		NFTBlockNumber: blockNoInt,
		NFTBlockHash:   strings.Split(txnDetails.BlockID, "-")[1],
		TransactionID:  txnDetails.TransactionID,
		Network:        conensusRequest.Type,
		NFTValue:       nftInfo.TokenValue,
		DeployerDID:    did,
		OwnerDID:       nftInfo.OwnerDID,
		PledgeAmount:   consensusContractDetails.TotalRBTs,
		QuorumList:     extractQuorumDID(conensusRequest.QuorumList),
		PledgeInfo:     PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		Comments:       txnDetails.Comment,
		SCTokenHash:    "' '",
	}
	explorerErr := c.ec.ExplorerNFTDeploy(eTrans)
	if explorerErr != nil {
		c.log.Error("Failed to send NFT transaction to explorer ", "err", explorerErr)
	}

	c.log.Info("NFT Deployed successfully", "duration", dif)
	resp.Status = true
	msg := fmt.Sprintf("NFT Deployed successfully in %v with trnxid %v", dif, txnDetails.TransactionID)
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
	//check the nft token from the DB base
	nftToken, err := c.w.GetNFTToken(executeReq.NFT)
	if err != nil {
		c.log.Error("Failed to retrieve NFT Token details from storage", err)
		resp.Message = err.Error()
		return resp
	}

	//get the gensys block of the amrt contract token
	tokenType := c.TokenType(NFTString)
	gensysBlock := c.w.GetGenesisTokenBlock(executeReq.NFT, tokenType)
	if gensysBlock == nil {
		c.log.Debug("Genesis block is empty - NFT not synced")
		resp.Message = "Genesis block is empty - NFT not synced"
		return resp
	}
	latestBlock := c.w.GetLatestTokenBlock(executeReq.NFT, tokenType)
	currentOwner := latestBlock.GetOwner()
	c.log.Info("The current owner of the NFT is :", currentOwner)

	// if currentOwner != executeReq.Executor {
	// 	c.log.Error("NFT not owned by the executor")
	// 	resp.Message = "NFT not owned by the executor"
	// 	return resp
	// }

	var metadata string = nftToken.Metadata
	var filename string = nftToken.Filename
	var receiver string
	var currentNFTValue float64

	// Empty Receiver indicates Self-Execution. Set the receiver to owner
	// and pledge value is set to current NFT value

	if executeReq.Receiver == "" {
		currentNFTValue = nftToken.TokenValue
		receiver = nftToken.DID
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
	consensusRequest := &ConensusRequest{
		ReqID:            uuid.New().String(),
		Type:             executeReq.QuorumType,
		ExecuterPeerID:   c.peerID,
		ContractBlock:    consensusContract.GetBlock(),
		NFT:              executeReq.NFT,
		Mode:             NFTExecuteMode,
		TransactionEpoch: txEpoch,
	}

	txnDetails, _, pds, err := c.initiateConsensus(consensusRequest, consensusContract, didCryptoLib)
	if err != nil {
		c.log.Error("Consensus failed", "err", err)
		resp.Message = "Consensus failed" + err.Error()
		return resp
	}

	var local bool
	if executeReq.Receiver != "" {
		receiverInfo, err := c.GetPeerDIDInfo(executeReq.Receiver)
		if err != nil {
			c.log.Error("Failed to get receiver peer info", "err", err)
			resp.Message = "Failed to get receiver peer info for " + executeReq.Receiver
			return resp
		}

		local = false
		if receiverInfo.PeerID == c.peerID {
			local = true
		}
	}

	err = c.w.UpdateNFTStatus(executeReq.NFT, wallet.TokenIsTransferred, local, executeReq.Receiver, executeReq.NFTValue)
	if err != nil {
		c.log.Error("Failed to update NFT status after transferring", err)
	}

	newEvent := model.NFTEvent{
		NFT:         consensusRequest.NFT,
		ExecutorDid: executeReq.Executor,
		ReceiverDid: receiver,
		NFTValue:    currentNFTValue,
		NFTMetadata: metadata,
		NFTFileName: filename,
	}

	err = c.publishNewNftEvent(&newEvent)
	if err != nil {
		c.log.Error("Failed to publish NFT executed  info")
	}

	et := time.Now()
	dif := et.Sub(st)
	txnDetails.TotalTime = float64(dif.Milliseconds())

	c.w.AddTransactionHistory(txnDetails)
	blockNoPart := strings.Split(txnDetails.BlockID, "-")[0]
	// Convert the string part to an int
	blockNoInt, _ := strconv.Atoi(blockNoPart)
	//Rename : TODO
	eTrans := &ExplorerNFTExecute{
		NFT:            executeReq.NFT,
		ExecutorDID:    executeReq.Executor,
		ReceiverDID:    receiver,
		Network:        executeReq.QuorumType,
		Comments:       executeReq.Comment,
		NFTValue:       executeReq.NFTValue,
		NFTData:        executeReq.NFTData,
		NFTBlockNumber: blockNoInt,
		NFTBlockHash:   strings.Split(txnDetails.BlockID, "-")[1],
		PledgeAmount:   consensusContractDetails.TotalRBTs,
		TransactionID:  txnDetails.TransactionID,
		QuorumList:     extractQuorumDID(consensusRequest.QuorumList),
		PledgeInfo:     PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		SCTokenHash:    "' '",
		Amount:         executeReq.NFTValue,
	}

	explorerErr := c.ec.ExplorerNFTTransaction(eTrans)
	if explorerErr != nil {
		c.log.Error("Failed to send NFT transaction to explorer ", "err", explorerErr)
	}

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

	fetchNFTResponse := c.FetchNFT(&fetchNFT)
	if !fetchNFTResponse.Status {
		c.log.Error("failed to fetch NFT: ", fetchNFTResponse.Message)
		return
	}

	// Sync Token Chain

	executorDid := newEvent.ExecutorDid
	receiverDid := newEvent.ReceiverDid

	nftTokenType := c.TokenType(NFTString)
	publisherAddress := peerID + "." + executorDid
	publisherPeer, err := c.getPeer(publisherAddress)
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
	}

	var tokenStatus int
	// Add check for receiverDid . In case of self-execution, it will be empty
	if !c.w.IsDIDExist(receiverDid) {
		tokenStatus = wallet.TokenIsTransferred
	} else {
		tokenStatus = wallet.TokenIsFree
	}

	err = c.w.CreateNFT(&wallet.NFT{TokenID: nft, DID: currentOwner, TokenStatus: tokenStatus, TokenValue: newEvent.NFTValue, Metadata: newEvent.NFTMetadata, Filename: newEvent.NFTFileName}, c.w.IsNFTExists(nft))
	if err != nil {
		c.log.Error("Failed to create NFT", "err", err)
		return
	}

	c.log.Info("Token chain of " + nft + " syncing successful")
}

func (c *Core) FetchNFT(fetchNFTRequest *FetchNFTRequest) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	nftJSON, err := c.ipfs.Cat(fetchNFTRequest.NFT)
	if err != nil {
		c.log.Error("Failed to get NFT from network", "err", err)
		basicResponse.Message = "Failed to get NFT details from network"
		return basicResponse
	}
	nftJSONBytes, err := io.ReadAll(nftJSON)
	if err != nil {
		c.log.Error("Failed to read NFT from network", "err", err)
		basicResponse.Message = "Failed to read NFT from network"
		return basicResponse
	}
	nftJSON.Close()

	var nft NFTIpfsInfo
	err = json.Unmarshal(nftJSONBytes, &nft)
	if err != nil {
		c.log.Error("Failed to parse nft", "err", err)
		basicResponse.Message = "Failed to parse nft"
		return basicResponse
	}
	err = c.GetNFTFromIpfs(fetchNFTRequest.NFT, nft.ArtifactHash)
	if err != nil {
		c.log.Error("failed to fetch NFT files from IPFS", "err", err)
		basicResponse.Message = "Failed to fetch NFT files from IPFS"
		return basicResponse
	}
	// Set the response values
	basicResponse.Status = true
	basicResponse.Message = "NFT fetched successfully"
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
		nftDetails = append(
			nftDetails,
			model.NFTInfo{
				NFTId:    nft.TokenID,
				Owner:    nft.DID,
				Value:    nft.TokenValue,
				Metadata: nft.Metadata,
				FileName: nft.Filename,
			})
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
		nftDetails = append(nftDetails, model.NFTInfo{NFTId: nft.TokenID, Owner: nft.DID, Value: nft.TokenValue, Metadata: nft.Metadata, FileName: nft.Filename})
	}
	response.NFTs = nftDetails
	response.Status = true
	response.Message = "Got All NFTs"

	return response

}

func (c *Core) CheckNFTFolderExists(nft string) (string, error) {
	dirPath := c.cfg.DirPath + "NFT/" + nft
	_, err := os.Stat(dirPath)
	if err == nil {
		return dirPath, nil // Folder exists
	}
	if os.IsNotExist(err) {
		return "", nil // Folder does not exist
	}
	return "", err // Some other error occurred
}
