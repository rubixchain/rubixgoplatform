package core

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

// type TransferToDidReq struct {
// 	Did          string
// 	ContractHash string
// 	CreatorDID   string
// 	FTName       string
// }

type ContractData struct {
	CreatorDID   string
	FTName       string
	SenderDID    string
	ReceiverDID  string
	SenderPeerId string // This won't be needed most probably
	TokenHashes  []string
}

func (c *Core) TransferToDidRequest(reqID string, req model.TransferToDidReq) {
	br := c.transferToDid(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br

}

func (c *Core) transferToDid(reqID string, req model.TransferToDidReq) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}
	dataReq := model.SmartContractTokenChainDataReq{
		Token:  req.ContractHash,
		Latest: false,
	}
	smartContractData := c.GetSmartContractTokenChainData(&dataReq)
	dataReply := smartContractData.SCTDataReply
	var requiredData string
	var contractData ContractData
	for _, reply := range dataReply {
		requiredData = reply.SmartContractData
		err := json.Unmarshal([]byte(requiredData), &contractData)
		if err != nil {
			fmt.Println("Error unmarshalling JSON:", err)
			return resp
		}

		fmt.Println(requiredData)
	}
	dc, err1 := c.SetupDID(reqID, contractData.SenderDID)
	if err1 != nil || dc == nil {
		c.log.Error("Failed to setup DID")
		return resp
	}

	// var creatorDID string
	// if req.CreatorDID == "" {
	// 	// Checking for same FTs with different creators
	// 	info, err := c.GetFTInfo(req.Did)
	// 	if err != nil || info == nil {
	// 		c.log.Error("Failed to get FT info for transfer", "err", err)
	// 		resp.Message = "Failed to get FT info for transfer"
	// 		return resp
	// 	}
	// 	ftNameToCreators := make(map[string][]string)
	// 	for _, ft := range info {
	// 		ftNameToCreators[ft.FTName] = append(ftNameToCreators[ft.FTName], ft.CreatorDID)
	// 	}
	// 	for ftName, creators := range ftNameToCreators {
	// 		if len(creators) > 1 {
	// 			c.log.Error(fmt.Sprintf("There are same FTs '%s' with different creators.", ftName))
	// 			for i, creator := range creators {
	// 				c.log.Error(fmt.Sprintf("Creator DID %d: %s", i+1, creator))
	// 			}
	// 			c.log.Info("Use -creatorDID flag to specify the creator DID and can proceed for transfer")
	// 			resp.Message = "There are same FTs with different creators, use -creatorDID flag to specify creatorDID"
	// 			return resp
	// 		}
	// 	}
	// 	creatorDID = info[0].CreatorDID
	// }
	// var AllFTs []wallet.FTToken
	var err error

	AllFTs, err := c.w.GetFTDetailsByTokenIds(contractData.TokenHashes)
	if err != nil {
		c.log.Error("Failed to get the ft token details of the provided tokenIds")
		return resp
	}
	fmt.Println("The list of FT details extracted from the DB :", AllFTs)
	// if req.CreatorDID != "" {
	// AllFTs, err = c.w.GetFreeFTsByNameAndCreatorDID(req.FTName, req.Did, req.CreatorDID)
	// 	creatorDID = req.CreatorDID
	// } else {
	// 	AllFTs, err = c.w.GetFreeFTsByNameAndDID(req.FTName, req.Did)
	// }
	// AvailableFTCount := len(AllFTs)
	// if err != nil {
	// 	c.log.Error("Failed to get FTs", "err", err)
	// 	resp.Message = "Insufficient FTs or FTs are locked or " + err.Error()
	// 	return resp
	// } else {
	// 	if req.FTCount > AvailableFTCount {
	// 		c.log.Error(fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount))
	// 		resp.Message = fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
	// 		return resp
	// 	}
	// }
	// FTsForTxn := AllFTs[:req.FTCount]
	//TODO: Pinning of tokens

	rpeerid := c.w.GetPeerID(contractData.ReceiverDID)
	if rpeerid == "" {
		// Check if DID is present in the DIDTable as the
		// receiver might be part of the current node
		_, err := c.w.GetDID(contractData.ReceiverDID)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				c.log.Error("Peer ID not found", "did", contractData.ReceiverDID)
				resp.Message = "invalid address, Peer ID not found"
				return resp
			} else {
				c.log.Error(fmt.Sprintf("Error occured while fetching DID info from DIDTable for DID: %v, err: %v", contractData.ReceiverDID, err))
				resp.Message = fmt.Sprintf("Error occured while fetching DID info from DIDTable for DID: %v, err: %v", contractData.ReceiverDID, err)
				return resp
			}
		} else {
			// Set the receiverPeerID to self Peer ID
			rpeerid = c.peerID
		}
	} else {
		receiverPeerID, err := c.getPeer(contractData.ReceiverDID, "")
		if err != nil {
			resp.Message = "Failed to get receiver peer, " + err.Error()
			return resp
		}
		defer receiverPeerID.Close()
	}

	// FTTokenIDs := make([]string, 0)
	// for i := range FTsForTxn {
	// 	FTTokenIDs = append(FTTokenIDs, FTsForTxn[i].TokenID)
	// }
	TokenInfo := make([]contract.TokenInfo, 0)
	for i := range AllFTs {
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(AllFTs[i].TokenID, tt)
		if blk == nil {
			c.log.Error("failed to get latest block, invalid token chain")
			resp.Message = "failed to get latest block, invalid token chain"
			return resp
		}
		bid, err := blk.GetBlockID(AllFTs[i].TokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		ti := contract.TokenInfo{
			Token:      AllFTs[i].TokenID,
			TokenType:  tt,
			TokenValue: AllFTs[i].TokenValue,
			OwnerDID:   req.ContractHash, // Here the owner did is given as the contract hash because the tokens were transferred from the senderdid to the contract hash
			BlockID:    bid,
		}
		TokenInfo = append(TokenInfo, ti)
	}
	b := c.w.GetGenesisTokenBlock(req.ContractHash, c.TokenType(SmartContractString))
	scValue, err5 := b.GetSmartContractValue(req.ContractHash)
	if err5 != nil {
		c.log.Error("Failed to get smart contract value", "err", err5)
		resp.Message = "Failed to get smart contract value, " + err5.Error()
		return resp
	}
	sct := &contract.ContractType{
		Type:       contract.SCFTType,
		PledgeMode: contract.PeriodicPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:    contractData.SenderDID,
			ReceiverDID:  contractData.ReceiverDID,
			ExecutorDID:  contractData.SenderDID,
			Comment:      fmt.Sprintf("Transferring FTs from contract : %s to did : %s ", req.ContractHash, contractData.ReceiverDID),
			TransTokens:  TokenInfo,
			ContractHash: req.ContractHash,
		},
		ReqID:     reqID,
		TotalRBTs: scValue,
	}
	FTData := model.FTInfo{
		FTName:     req.FTName,
		FTCount:    len(contractData.TokenHashes),
		CreatorDID: req.CreatorDID,
	}
	fmt.Println("The FTData to be sent to the consensus is :", FTData)
	fmt.Println("The FTData in split format is :", FTData.FTName, FTData.FTCount)
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		Mode:               FTContractToDidTransferMode,
		ReqID:              uuid.New().String(),
		Type:               2,
		SenderPeerID:       c.peerID,
		ReceiverPeerID:     rpeerid,
		ContractBlock:      sc.GetBlock(),
		FTinfo:             FTData,
		SmartContractToken: req.ContractHash,
	}
	td, _, pds, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}
	et := time.Now()
	dif := et.Sub(st)
	td.Amount = float64(len(contractData.TokenHashes))
	td.TotalTime = float64(dif.Milliseconds())
	c.w.AddTransactionHistory(td)
	AllTokens := make([]AllToken, len(AllFTs))
	for i := range AllFTs {
		tokenDetail := AllToken{}
		tokenDetail.TokenHash = AllFTs[i].TokenID
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(AllFTs[i].TokenID, tt)
		bid, _ := blk.GetBlockID(AllFTs[i].TokenID)

		blockNoPart := strings.Split(bid, "-")[0]
		// Convert the string part to an int
		blockNoInt, err := strconv.Atoi(blockNoPart)
		if err != nil {
			log.Printf("Error getting BlockID: %v", err)
			continue
		}
		tokenDetail.BlockNumber = blockNoInt
		tokenDetail.BlockHash = strings.Split(bid, "-")[1]

		AllTokens[i] = tokenDetail
	}

	eTrans := &ExplorerFTTrans{
		FTBlockHash:     AllTokens,
		CreatorDID:      contractData.CreatorDID,
		SenderDID:       contractData.SenderDID,
		ReceiverDID:     contractData.ReceiverDID,
		FTName:          req.FTName,
		FTTransferCount: len(contractData.TokenHashes),
		Network:         2,
		FTSymbol:        "TODO",
		Comments:        fmt.Sprintf("Transferring FTs from contract : %s to did : %s ", req.ContractHash, contractData.ReceiverDID),
		TransactionID:   td.TransactionID,
		PledgeInfo:      PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		QuorumList:      extractQuorumDID(cr.QuorumList),
		Amount:          AllFTs[0].TokenValue * float64(len(contractData.TokenHashes)),
		FTTokenList:     contractData.TokenHashes,
	}

	updateFTTableErr := c.UpdateFTTable(contractData.SenderDID)
	if updateFTTableErr != nil {
		c.log.Error("Failed to update FT table after transfer ", "err", updateFTTableErr)
		resp.Message = "Failed to update FT table after transfer"
		return resp
	}
	explorerErr := c.ec.ExplorerFTTransaction(eTrans)
	if explorerErr != nil {
		c.log.Error("Failed to send FT transaction to explorer ", "err", explorerErr)
	}
	c.log.Info("FT Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
	msg := fmt.Sprintf("FT Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
	resp.Status = true
	resp.Message = msg
	return resp
}
