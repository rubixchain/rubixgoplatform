package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, wholeToken int, continueCreation bool) {
	err := c.createFTs(reqID, ftname, ftcount, wholeToken, did, continueCreation)
	br := model.BasicResponse{
		Status:  true,
		Message: "FT created successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	channel := c.GetWebReq(reqID)
	if channel == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	channel.OutChan <- &br
}

func (c *Core) createFTs(reqID string, FTName string, numFTs int, numWholeTokens int, did string, continueCreation bool) error {
	if did == "" {
		c.log.Error("DID is empty")
		return fmt.Errorf("DID is empty")
	}
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !isAlphanumericDID || !strings.HasPrefix(did, "bafybmi") || len(did) != 59 {
		c.log.Error("Invalid FT creator's DID. Please provide valid DID")
		return fmt.Errorf("invalid DID, Please provide valid DID")
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil || dc == nil {
		c.log.Error("Failed to setup DID")
		return fmt.Errorf("DID crypto is not initialized, err: %v ", err)
	}

	var FT wallet.FT
	c.s.Read(wallet.FTStorage, &FT, "ft_name=? AND  creator_did=?", FTName, did)

	//If any one value exists in the table, stop reading
	//Existing FT check
	if FT != (wallet.FT{}) && !continueCreation {
		c.log.Error("FT Name already exists")
		return fmt.Errorf("FT Name already exists, use -continue flag to create more FTs with same name")
	}

	//If FT  name exists, then create FTs incrementally more

	// Validate input parameters
	switch {
	case numFTs <= 0:
		return fmt.Errorf("number of tokens to create must be greater than zero")
	case numWholeTokens <= 0:
		return fmt.Errorf("number of whole tokens must be a positive integer")
	case numFTs > int(numWholeTokens*1000):
		return fmt.Errorf("max allowed FT count is 1000 for 1 RBT")
	}

	// Fetch whole tokens (RBT Tokens) using GetToken
	//Locking the RBT Tokens to prevent any transactions on it
	wholeTokens, err := c.GetTokens(dc, did, float64(numWholeTokens), 0)
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}

	//TODO: Need to test and verify whether tokens are getting unlocked if there is an error in creating FT.
	defer c.w.ReleaseTokens(wholeTokens) // Will this be called after function?

	//Calculate the value of each FT
	fractionalValue, err := c.GetPreciseFractionalValue(int(numWholeTokens), numFTs)
	if err != nil {
		c.log.Error("Failed to calculate FT token value", err)
		return err
	}

	//Check and validate fractional value only if FT with the given name is already created
	if FT != (wallet.FT{}) && fractionalValue != float64(FT.FTValue) {
		c.log.Error("FT value is not same as previous FT value")
		minRBT := math.Ceil(float64(numFTs) * FT.FTValue)
		newFTCount := int(math.Ceil(minRBT) / FT.FTValue)
		return fmt.Errorf("FT value is not same as previous FT value, previous value of FT is %v. Minimum RBTs needed is %v to create total FTs of %v", FT.FTValue, minRBT, newFTCount)
	}

	//RBT Tokens used to create FTs
	parentTokenIDsArray := make([]string, len(wholeTokens))
	for i, token := range wholeTokens {
		parentTokenIDsArray[i] = token.TokenID
	}
	parentTokenIDs := strings.Join(parentTokenIDsArray, ",")

	newFTs := make([]wallet.FTToken, numFTs)
	newFTTokenIDs := make([]string, numFTs)

	numWorkers := runtime.NumCPU() * 2

	ftJobs := make(chan int, numFTs) // Channel to distribute tasks

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cancellation on return
	errChan := make(chan error, numWorkers)
	var tokenCounter uint64 // Ensures sequential FT numbering
	var once sync.Once

	var startingFTNum int
	if FT != (wallet.FT{}) {
		startingFTNum = FT.FTCountOriginal
	} else {
		startingFTNum = 0
	}
	//Worker Function
	worker := func(workerID int) {
		defer wg.Done()
		for range ftJobs {
			if ctx.Err() != nil {
				return
			}
			ftNum := int(atomic.AddUint64(&tokenCounter, 1)) - 1 + startingFTNum
			fmt.Println("FT count : ", ftNum)

			fmt.Println("Worker", workerID, "processing FT", ftNum)

			if err := processFT(c, dc, ftNum, newFTs, newFTTokenIDs, did, FTName, fractionalValue, parentTokenIDs, startingFTNum); err != nil {
				c.log.Error("FT processing failed", "err", err)
				once.Do(func() { cancel() })
				errChan <- err
				return

			}
		}
	}

	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go worker(w)
	}
	// Distribute FT jobs
	go func() {
		defer close(ftJobs)
		for i := 0; i < numFTs; i++ {
			if ctx.Err() != nil {
				return
			}
			ftJobs <- i
		} // Close earlier so workers don't block
	}()

	// **Wait for all workers to finish**
	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return err
	}

	//Adding levelDB blocks for RBT Tokens as well
	for i := range wholeTokens {
		fmt.Println("Is this getting called?")
		release := true
		defer c.relaseToken(&release, wholeTokens[i].TokenID)
		ptt := c.TokenType(RBTString)
		if wholeTokens[i].ParentTokenID != "" && wholeTokens[i].TokenValue < 1 {
			ptt = c.TokenType(RBTString)
		}
		// ptt := c.TokenType(ptts)

		bti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     wholeTokens[i].TokenID,
					TokenType: ptt,
				},
			},
			Comment: "Token burnt at : " + time.Now().String(),
		}
		tcb := &block.TokenChainBlock{
			TransactionType: block.TokenIsBurntForFT,
			TokenOwner:      did,
			TransInfo:       bti,
			TokenValue:      wholeTokens[i].TokenValue,
			ChildTokens:     newFTTokenIDs,
		}
		ctcb := make(map[string]*block.Block)
		ctcb[wholeTokens[i].TokenID] = c.w.GetLatestTokenBlock(wholeTokens[i].TokenID, ptt)
		block := block.CreateNewBlock(ctcb, tcb)
		if block == nil {
			return fmt.Errorf("failed to create new block")
		}
		err = block.UpdateSignature(dc)
		if err != nil {
			c.log.Error("FT creation failed, failed to update signature", "err", err)
			return err
		}
		err = c.w.AddTokenBlock(wholeTokens[i].TokenID, block)
		if err != nil {
			c.log.Error("FT creation failed, failed to add token block", "err", err)
			return err
		}
		wholeTokens[i].TokenStatus = wallet.TokenIsBurntForFT
		err = c.w.UpdateToken(&wholeTokens[i])
		if err != nil {
			c.log.Error("FT token creation failed, failed to update token status", "err", err)
			return err
		}
		release = false
	}

	//Updating the SQLite3 table
	for i := range newFTs {
		newFTs[i].CreatorDID = did
	}
	err = c.w.CreateFT(newFTs, numFTs)
	if err != nil {
		c.log.Error("Failed to write FT details in FT tokens table", "err", err)
		return err
	}
	if FT != (wallet.FT{}) {
		FT.FTCountAvailable += numFTs
		FT.FTCountOriginal += numFTs
		Err := c.s.Update(wallet.FTStorage, &FT, "ft_name=? AND creator_did=?", FTName, did)
		if Err != nil {
			c.log.Error("Failed to update FT:", FTName, "Error:", Err)
			return Err
		}
	} else {
		Ft := wallet.FT{FTName: FTName, FTCountOriginal: numFTs, FTCountAvailable: numFTs, CreatorDID: did, FTValue: fractionalValue}
		addErr := c.s.Write(wallet.FTStorage, &Ft)
		if addErr != nil {
			c.log.Error("Failed to add new FT:", Ft.FTName, "Error:", addErr)
			return addErr
		}
	}
	return nil
}

func processFT(c *Core, dc did.DIDCrypto, ftNum int, newFTs []wallet.FTToken, newFTTokenIDs []string, did, FTName string, fractionalValue float64, parentTokenIDs string, startingFTIndex int) error {
	ftID, err := c.w.Add(bytes.NewBufferString(FTName+" "+strconv.Itoa(ftNum)+" "+did), did, wallet.OwnerRole)
	if err != nil {
		c.log.Error("Failed to create FT, Failed to add token to IPFS", "err", err)
		return fmt.Errorf("failed to add token to IPFS: %v", err)
	}

	c.log.Info("FT created: " + ftID)
	newFTTokenIDs[ftNum-startingFTIndex] = ftID

	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     ftID,
				TokenType: c.TokenType(FTString),
			},
		},
		Comment: "FT generated at : " + time.Now().String() + " for FT Name : " + FTName, //Do we need timestamp?
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenGeneratedType,
		TokenOwner:      did,
		TransInfo:       bti,
		GenesisBlock: &block.GenesisBlock{
			Info: []block.GenesisTokenInfo{
				{
					Token:       ftID,
					ParentID:    parentTokenIDs,
					TokenNumber: ftNum,
				},
			},
		},
		TokenValue: fractionalValue,
	}
	ctcb := map[string]*block.Block{ftID: nil}
	block := block.CreateNewBlock(ctcb, tcb)
	if block == nil {
		return fmt.Errorf("failed to create new block")
	}
	if err := block.UpdateSignature(dc); err != nil {
		return fmt.Errorf("failed to update signature: %v", err)
	}
	if err := c.w.AddTokenBlock(ftID, block); err != nil {
		return fmt.Errorf("failed to add token chain block: %v", err)
	}
	newFTs[ftNum-startingFTIndex] = wallet.FTToken{TokenID: ftID, FTName: FTName, TokenStatus: wallet.TokenIsFree, TokenValue: fractionalValue, DID: did}
	return nil
}

func (c *Core) GetFTInfo(did string) ([]model.FTInfo, error) {
	if !c.w.IsDIDExist(did) {
		c.log.Error("DID does not exist")
		return nil, fmt.Errorf("DID does not exist")
	}
	FT, err := c.w.GetFTsAndCount(did)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens", "err", err)
		return []model.FTInfo{}, fmt.Errorf("failed to get tokens")
	}
	ftInfoMap := make(map[string]map[string]int)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]int) // Initialize map for each FTName
		}
		ftInfoMap[t.FTName][t.CreatorDID] += t.FTCountAvailable // Increment count for the specific CreatorDID
	}
	info := make([]model.FTInfo, 0)
	for ftName, creatorCounts := range ftInfoMap {
		for creatorDID, count := range creatorCounts {
			info = append(info, model.FTInfo{
				FTName:     ftName,
				FTCount:    count,
				CreatorDID: creatorDID,
			})
		}
	}
	return info, nil
}

func (c *Core) InitiateFTTransfer(reqID string, req *model.TransferFTReq) {
	br := c.initiateFTTransfer(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}

func (c *Core) initiateFTTransfer(reqID string, req *model.TransferFTReq) *model.BasicResponse {
	st := time.Now()
	resp := &model.BasicResponse{
		Status: false,
	}
	if req.Sender == req.Receiver {
		c.log.Error("Sender and receiver cannot same")
		resp.Message = "Sender and receiver cannot be same"
		return resp
	}
	if req.Sender == "" || req.Receiver == "" {
		c.log.Error("Sender and receiver cannot be empty")
		resp.Message = "Sender and receiver cannot be empty"
		return resp
	}
	isAlphanumericSender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Sender)
	isAlphanumericReceiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.Receiver)
	if !isAlphanumericSender || !isAlphanumericReceiver {
		c.log.Error("Invalid sender or receiver address. Please provide valid DID")
		resp.Message = "Invalid sender or receiver address. Please provide valid DID"
		return resp
	}
	if !strings.HasPrefix(req.Sender, "bafybmi") || len(req.Sender) != 59 || !strings.HasPrefix(req.Receiver, "bafybmi") || len(req.Receiver) != 59 {
		c.log.Error("Invalid sender or receiver DID")
		resp.Message = "Invalid sender or receiver DID"
		return resp
	}
	_, did, ok := util.ParseAddress(req.Sender)
	if !ok {
		c.log.Error("Failed to parse sender DID")
		resp.Message = "Invalid sender DID"
		return resp
	}

	_, rdid, ok := util.ParseAddress(req.Receiver)
	if !ok {
		c.log.Error("Failed to parse receiver DID")
		resp.Message = "Invalid receiver DID"
		return resp
	}

	if req.FTCount <= 0 {
		c.log.Error("Input transaction amount is less than minimum FT transaction amount")
		resp.Message = "Invalid FT count"
		return resp
	}
	if req.FTName == "" {
		c.log.Error("FT name cannot be empty")
		resp.Message = "FT name is required"
		return resp
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup DID")
		resp.Message = "Failed to setup DID, " + err.Error()
		return resp
	}
	var creatorDID string
	if req.CreatorDID == "" {
		// Checking for same FTs with different creators, can be checked in the FTTable
		var FT []wallet.FT
		err = c.s.Read(wallet.FTStorage, &FT, "ft_name=?", req.FTName)
		if err != nil || FT == nil {
			c.log.Error("Failed to get FT info for transfer", "err", err)
			resp.Message = "Failed to get FT info for transfer"
			return resp
		}

		if len(FT) > 1 {
			c.log.Error(fmt.Sprintf("Multiple creators found: There are same FTs with name %s created by different creators, use -creatorDID flag to specify creatorDID", req.FTName))
			resp.Message = "There are same FTs with name" + req.FTName + " created by different creators, use -creatorDID flag to specify creatorDID"
			return resp
		}

		creatorDID = FT[0].CreatorDID
	} else {
		creatorDID = req.CreatorDID
	}
	c.log.Info("getting FT Tokens")
	AllFTs, err := c.w.GetFreeFTsByNameAndCreatorDID(req.FTName, did, creatorDID)
	if err != nil {
		c.log.Error("Failed to get FTs", "err", err)
		resp.Message = "Insufficient FTs or FTs are locked or " + err.Error()
		return resp
	}
	AvailableFTCount := len(AllFTs)

	if req.FTCount > AvailableFTCount {
		errMsg := fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
		c.log.Error(errMsg)
		resp.Message = errMsg
		return resp
	}
	//TODO: Pinning of tokens

	// Fetching peer's peer id
	peerInfo, err := c.GetPeerDIDInfo(req.Receiver)
	if err != nil {
		if peerInfo == nil {
			c.log.Error("could not get peerId of receiver ", req.Receiver, "error", err)
			resp.Message = fmt.Sprintf("could not get peerId of receiver : %v, error: %v", req.Receiver, err)
			return resp
		}
		if strings.Contains(err.Error(), "retry") {
			c.AddPeerDetails(*peerInfo)
		}
	}
	if peerInfo.PeerID == "" {
		c.log.Error("failed to get peerId of receiver ", req.Receiver, "error", err)
		resp.Message = fmt.Sprintf("failed to get peerId of receiver : %v, error: %v", req.Receiver, err)
		return resp
	}

	receiverPeerID, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer receiverPeerID.Close()

	FTsForTxn := AllFTs[:req.FTCount]
	FTTokenIDs := make([]string, req.FTCount)
	TokenInfo := make([]contract.TokenInfo, req.FTCount)

	tt := c.TokenType(FTString)

	for i := range FTsForTxn {
		tokenID := FTsForTxn[i].TokenID
		blk := c.w.GetLatestTokenBlock(tokenID, tt)
		if blk == nil {
			errMsg := fmt.Sprintf("failed to get latest block for token %v, invalid token chain", tokenID)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return resp
		}
		bid, err := blk.GetBlockID(tokenID)
		if err != nil {
			c.log.Error("failed to get block id", "err", err)
			resp.Message = "failed to get block id, " + err.Error()
			return resp
		}
		TokenInfo[i] = contract.TokenInfo{
			Token:      tokenID,
			TokenType:  tt,
			TokenValue: FTsForTxn[i].TokenValue,
			OwnerDID:   did,
			BlockID:    bid,
		}
		FTTokenIDs[i] = tokenID
	}
	sct := &contract.ContractType{
		Type:       contract.SCFTType,
		PledgeMode: contract.PeriodicPledgeMode,
		TransInfo: &contract.TransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: TokenInfo,
		},
		ReqID: reqID,
	}
	FTData := model.FTInfo{
		FTName:     req.FTName,
		FTCount:    req.FTCount,
		CreatorDID: creatorDID,
	}
	sc := contract.CreateNewContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		Mode:           FTTransferMode,
		ReqID:          uuid.New().String(),
		Type:           req.QuorumType,
		SenderPeerID:   c.peerID,
		ReceiverPeerID: peerInfo.PeerID,
		ContractBlock:  sc.GetBlock(),
		FTinfo:         FTData,
	}
	td, _, pds, err := c.initiateConsensus(cr, sc, dc)
	if err != nil {
		c.log.Error("Consensus failed ", "err", err)
		resp.Message = "Consensus failed " + err.Error()
		return resp
	}

	dif := time.Since(st).Milliseconds()
	td.Amount = float64(req.FTCount)
	td.TotalTime = float64(dif)
	c.w.AddTransactionHistory(td)

	AllTokens := make([]AllToken, len(FTsForTxn))
	for i := range FTsForTxn {
		tokenDetail := AllToken{}
		tokenDetail.TokenHash = FTsForTxn[i].TokenID
		tt := c.TokenType(FTString)
		blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
		bid, _ := blk.GetBlockID(FTsForTxn[i].TokenID)

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
		CreatorDID:      creatorDID,
		SenderDID:       did,
		ReceiverDID:     rdid,
		FTName:          req.FTName,
		FTTransferCount: req.FTCount,
		Network:         req.QuorumType,
		FTSymbol:        "TODO",
		Comments:        req.Comment,
		TransactionID:   td.TransactionID,
		PledgeInfo:      PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
		QuorumList:      extractQuorumDID(cr.QuorumList),
		Amount:          FTsForTxn[0].TokenValue * float64(req.FTCount),
		FTTokenList:     FTTokenIDs,
	}

	updateFTTableErr := c.updateFTTable(did, req.FTName, creatorDID)
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

func (c *Core) GetPreciseFractionalValue(a, b int) (float64, error) {
	if b == 0 || a == 0 {
		return 0, errors.New("RBT value or FT count should not be zero")
	}
	result := float64(a) / float64(b)
	decimalPlaces := len(strconv.FormatFloat(result, 'f', -1, 64)) - 2 // Subtract 2 for "0."

	if decimalPlaces > 3 {
		// Find the nearest possible value for b by checking from b-10 to b+10
		var nearestB int
		minDiff := math.MaxFloat64
		found := false

		for i := b - 10; i <= b+10; i++ {
			if i <= 0 {
				continue // Skip non-positive values of b
			}
			tempResult := float64(a) / float64(i)
			tempDecimalPlaces := len(strconv.FormatFloat(tempResult, 'f', -1, 64)) - 2

			if tempDecimalPlaces <= 3 {
				diff := math.Abs(result - tempResult)
				if diff < minDiff {
					minDiff = diff
					nearestB = i
					found = true
				}
			}
		}

		if found {
			return 0, fmt.Errorf("FT value exceeds 3 decimal places, nearest possible value for FT count is %d", nearestB)
		} else {
			return 0, fmt.Errorf("FT value exceeds 3 decimal places, no suitable b found within range")
		}
	}
	return result, nil
}

func (c *Core) updateFTTable(did string, ftName string, creatorDID string) error {
	fmt.Println("Updating FT table for DID:", did, "FT Name:", ftName, "Creator DID:", creatorDID)
	// AllFTs, err := c.w.GetFTsAndCount(did)
	// Get count of Free FTs for a given DID (as owner) and FT Name
	// For Sender side :
	// If no records are found, check if the creatorDID is same as the ownerDID, if yes, then the creatorDID can create more FTs, and no need to remove the record.
	// If the creatorDID is different than ownerDID that means, the sender was just a owner for this FT, and the record can be removed.

	var FT []wallet.FTToken
	var FTInfo wallet.FT

	err := c.s.Read(wallet.FTTokenStorage, &FT, "owner_did=? AND ft_name=? AND creator_did=? AND token_status=?", did, ftName, creatorDID, wallet.TokenIsFree)

	if err != nil {
		readErr := fmt.Sprint(err)
		if strings.Contains(readErr, "no records found") {

			if did != creatorDID {
				err = c.s.Delete(wallet.FTStorage, &wallet.FT{}, "ft_name=? AND creator_did=?", ftName, creatorDID)
				if err != nil {
					c.log.Error("Failed to delete FT:", ftName, "Error:", err)
					return err
				}
				return nil

			} else {
				err = c.s.Read(wallet.FTStorage, &FTInfo, "ft_name=? AND creator_did=?", ftName, creatorDID)
				if err != nil {
					return err
				}
				FTInfo.FTCountAvailable = 0
				err = c.s.Update(wallet.FTStorage, &FTInfo, "ft_name=? AND creator_did=?", ftName, creatorDID)
				if err != nil {
					c.log.Error("Failed to update FT:", ftName, "Error:", err)
					return err
				}
				return nil
			}
		} else {
			c.log.Error("Failed to get FTs", "err", err)
			return err
		}
	}

	// If records are found in FT Token Table, then update the FT table

	// Case 1. Read FT Table, if no records found, then add new record. This means a receiver is trying to update the table

	err = c.s.Read(wallet.FTStorage, &FTInfo, "ft_name=? AND creator_did=?", ftName, creatorDID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			FTInfo = wallet.FT{
				FTName:           ftName,
				FTCountAvailable: len(FT),
				FTCountOriginal:  0, //as the owner did is not the creator
				CreatorDID:       creatorDID,
				FTValue:          FT[0].TokenValue,
			}
			addErr := c.s.Write(wallet.FTStorage, &FTInfo)
			if addErr != nil {
				c.log.Error("Failed to add new FT:", ftName, "Error:", addErr)
				return addErr
			}
			return nil
		} else {
			c.log.Error("Failed to read FT Table", "err", err)
			return err
		}
	}

	// Case 2. If records are found in FT Table, then update the FT table with new available FT Count
	FTInfo.FTCountAvailable = len(FT)
	err = c.s.Update(wallet.FTStorage, &FTInfo, "ft_name=? AND creator_did=?", ftName, creatorDID)
	if err != nil {
		c.log.Error("Failed to update FT:", ftName, "Error:", err)
		return err
	}

	return nil
}
