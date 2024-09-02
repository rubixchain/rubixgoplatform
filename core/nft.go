package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type CreateNFTRequest struct {
	NFTFilePath string
	DID         string
}

type deployNFTRequest struct {
	nft        string
	did        string
	quorumType int
}

func (c *Core) CreateNFTRequest(requestID string, createNFTRequest *CreateNFTRequest) *model.BasicResponse {

	//	defer os.RemoveAll(smartContractTokenRequest.SCPath)
	dc, err := c.SetupDID(reqID, createNFTRequest.DID)
	if err != nil {
		c.log.Error("Failed to setup DID")
	}

	createNFTResponse := c.createNFT(requestID, createNFTRequest, dc)
	didChannel := c.GetWebReq(requestID)
	if dc == nil {
		c.log.Error("failed to get web request", "requestID", requestID)
		return nil
	}
	didChannel.OutChan <- createNFTResponse

	return createNFTResponse
}

func (c *Core) createNFT(requestID string, createNFTRequest *CreateNFTRequest, dc did.DIDCrypto) *model.BasicResponse {
	basicResponse := &model.BasicResponse{
		Status: false,
	}

	nftFile, err := os.Open(createNFTRequest.NFTFilePath)
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

	nft := rac.RacType{
		Type:      c.TokenType(NFTString),
		DID:       createNFTRequest.DID,
		CreatorID: createNFTRequest.DID,
		TimeStamp: time.Now().String(),
		NFTInfo: &rac.RacNFTInfo{
			Creator:            createNFTRequest.DID,
			ContentHash:        nftFileHash,
			ContentDescription: "", //Content Description to be added
		},
	}
	racBlocks, err := rac.CreateRac(&nft)
	if err != nil {
		c.log.Error("Failed to create rac", "err", err)
		return basicResponse
	}

	if len(racBlocks) != 1 {
		c.log.Error("Failed to create RAC NFT block")
		return basicResponse
	}

	// Update the signature of the RAC block
	err = racBlocks[0].UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update DID signature in RAC NFT Block", "err", err)
		return basicResponse
	}

	// smartContractTokenJSON, err := json.MarshalIndent(smartContractToken, "", "  ")
	// if err != nil {
	// 	c.log.Error("Failed to marshal SmartContractToken struct", "err", err)
	// 	return basicResponse
	// }
	nftData, err := json.Marshal(racBlocks)
	if err != nil {
		c.log.Error("Failed to marshal RAC NFT block", "err", err)
	}
	nftTokenHash, err := c.ipfs.Add(bytes.NewReader(nftData))
	if err != nil {
		c.log.Error("Failed to add SmartContractToken to IPFS", "err", err)
		return basicResponse
	}

	c.log.Info("The nft token hash generated ", nftTokenHash)

	// Set the response status and message
	nftTokenResponse := &SmartContractTokenResponse{
		Message: "NFT Token generated successfully",
		Result:  nftTokenHash,
	}
	fmt.Println("NFTResponse : ", nftTokenResponse)
	// _, err = c.RenameSCFolder(smartContractTokenRequest.SCPath, smartContractTokenHash)
	// if err != nil {
	// 	c.log.Error("Failed to rename SC folder", "err", err)
	// 	return basicResponse
	// }
	//err = c.w.CreateSmartContractToken(&wallet.SmartContract{SmartContractHash: smartContractTokenHash, Deployer: smartContractTokenRequest.DID, BinaryCodeHash: binaryCodeHash, RawCodeHash: rawCodeHash, SchemaCodeHash: schemaCodeHash, ContractStatus: 6})
	nftTokenDetails := wallet.NFT{
		TokenID:     nftTokenHash,
		DID:         nft.DID,
		TokenStatus: 0,
		TokenValue:  0,
	}
	c.w.CreateNFT(&nftTokenDetails) //To be done : Write the created token details onto the db
	// Set the response values
	// basicResponse.Status = true
	// basicResponse.Message = smartContractTokenResponse.Message
	// basicResponse.Result = smartContractTokenResponse

	return basicResponse
}

func (c *Core) deployNFT(reqID string, deployReq deployNFTRequest) *model.BasicResponse {
	st := time.Now()
	txEpoch := int(st.Unix())

	resp := &model.BasicResponse{
		Status: false,
	}
	_, did, ok := util.ParseAddress(deployReq.did)
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
	nft, err := c.w.GetNFT(did, deployReq.nft, false)
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
		Token:      deployReq.nft,
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
			NFT:         deployReq.nft,
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
		Type:             deployReq.quorumType,
		DeployerPeerID:   c.peerID,
		ContractBlock:    consensusContract.GetBlock(),
		NFT:              deployReq.nft,
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
