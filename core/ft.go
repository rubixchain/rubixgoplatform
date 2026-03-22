package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"runtime"
	"sync"
	"sync/atomic"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"

	rubixmath "github.com/rubixchain/rubixgoplatform/math"
)

func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, wholeToken int, ftNumStartIndex int) {
	err := c.createFTs(reqID, ftname, ftcount, wholeToken, did, ftNumStartIndex)
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

func (c *Core) createFTs(reqID string, FTName string, numFTs int, numWholeTokens int, did string, ftNumStartIndex int) error {
	if did == "" {
		c.log.Error("DID is empty")
		return fmt.Errorf("DID is empty")
	}
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !isAlphanumericDID || !strings.HasPrefix(did, "bafybmi") || len(did) != 59 {
		c.log.Error("Invalid FT creator's DID. Please provide valid DID")
		return fmt.Errorf("Invalid DID, Please provide valid DID")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil || dc == nil {
		c.log.Error("Failed to setup DID")
		return fmt.Errorf("DID crypto is not initialized, err: %v ", err)
	}

	// var FT []wallet.FT

	// c.s.Read(wallet.FTStorage, &FT, "ft_name=? AND creator_did=?", FTName, did)

	// if len(FT) != 0 {
	// 	c.log.Error("FT Name already exists")
	// 	return fmt.Errorf("FT Name already exists")
	// }

	// Validate input parameters

	switch {
	case numFTs <= 0:
		return fmt.Errorf("number of tokens to create must be greater than zero")
	case numWholeTokens <= 0:
		return fmt.Errorf("number of whole tokens must be a positive integer")
	case numFTs > int(numWholeTokens*1000):
		return fmt.Errorf("max allowed FT count is 1000 for 1 RBT")
	}

	// Pre-fetch locked tokens and denomination map for pure CollectRBTTokens
	lockedTokens, err := c.w.LockTokensForSplit(context.Background(), did, rubixmath.FloatPrecision(float64(numWholeTokens)))
	if err != nil {
		c.log.Error("Failed to lock tokens for FT split", "err", err)
		return fmt.Errorf("failed to lock tokens for FT split: %w", err)
	}
	denomMap, err := c.w.GetTokenDenomArray(did)
	if err != nil {
		c.log.Error("Failed to get token denom array", "err", err)
		return fmt.Errorf("failed to get token denom array: %w", err)
	}

	// Fetch whole tokens
	wholeTokens, _, _, _, err := parts.CollectRBTTokens(
		dc, c.w, rubixmath.FloatPrecision(float64(numWholeTokens)),
		lockedTokens, denomMap, constants.NetworkMode_Testnet, c.log,
	)
	if err != nil || wholeTokens == nil {
		c.log.Error("Failed to fetch whole token for FT creation")
		return err
	}
	fractionalValue, err := c.GetPresiceFractionalValue(int(numWholeTokens), numFTs)
	if err != nil {
		c.log.Error("Failed to calculate FT token value", err)
		return err
	}

	var parentTokenIDsArray []string
	for _, token := range wholeTokens {
		parentTokenIDsArray = append(parentTokenIDsArray, token.TokenID)
	}
	parentTokenIDs := strings.Join(parentTokenIDsArray, ",")

	type ftJob struct {
		Index int
	}
	type ftResult struct {
		FTToken wallet.FTToken
		FTID    string
		Err     error
	}

	numWorkers := runtime.NumCPU()
	jobs := make(chan ftJob, numFTs)
	results := make(chan ftResult, numFTs)
	var wg sync.WaitGroup

	var completed int32
	var lastLoggedPercent int32

	// Prepare to collect provider details for batch write
	providerMaps := make([]models.TokenProviderMap, 0, numFTs)
	// Mutex for providerMaps slice
	c.log.Info("Initializing FT creation: progress logging")
	currentTime := time.Now()

	ftTokenTypeID := int16(models.GetTokenTypeID(constants.TokenType_FT))
	mintRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Mint))

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			i := job.Index

			ftnumString := strconv.Itoa(i)
			parts := []string{FTName, ftnumString, did}
			ftID := strings.Join(parts, "_")

			// Collect provider map for batch
			// Use mutex to avoid race condition
			// providerMapMutex.Lock()
			// providerMaps = append(providerMaps, tpm)
			// providerMapMutex.Unlock()
			// Progress logging (remove per-token log)
			newCount := atomic.AddInt32(&completed, 1)
			currentPercent := int32(math.Floor(float64(newCount*100) / float64(numFTs)))
			if currentPercent%10 == 0 && atomic.LoadInt32(&lastLoggedPercent) < currentPercent {
				if atomic.CompareAndSwapInt32(&lastLoggedPercent, lastLoggedPercent, currentPercent) {
					c.log.Info(fmt.Sprintf("FT creation progress: %d%% (%d/%d created)", currentPercent, newCount, numFTs))
				}
			}

			// Build genesis transaction info for FT token
			txInfo := &models.TransactionInfo{
				Initiator: did,
				Owner:     did,
				Epoch:     int(currentTime.Unix()),
				Network:   constants.NetworkID_RBT_Testnet,
				Tokens: &models.TransactionTokens{
					FT: []*models.TokenInfo{
						{TokenID: ftID, PreviousTransactionID: parentTokenIDs},
					},
				},
			}
			infoBytes, err := models.SerializeTransactionInfo(txInfo)
			if err != nil {
				results <- ftResult{Err: fmt.Errorf("createFTs: failed to serialize transaction info for FT %s: %w", ftID, err)}
				continue
			}
			signatureBytes, err := dc.PvtSign(infoBytes)
			if err != nil {
				results <- ftResult{Err: fmt.Errorf("createFTs: failed to sign transaction for FT %s: %w", ftID, err)}
				continue
			}
			sigStruct := &models.Signature{InitiatorSignature: hex.EncodeToString(signatureBytes)}
			sigBytes, err := json.Marshal(sigStruct)
			if err != nil {
				results <- ftResult{Err: fmt.Errorf("createFTs: failed to marshal signature for FT %s: %w", ftID, err)}
				continue
			}
			txID, err := wallet.ComputeTransactionID(txInfo)
			if err != nil {
				results <- ftResult{Err: fmt.Errorf("createFTs: failed to compute transaction ID for FT %s: %w", ftID, err)}
				continue
			}

			genesisTx := &models.Transactions{
				ID:        txID,
				Info:      infoBytes,
				Signature: json.RawMessage(sigBytes),
			}
			t := &models.Token{
				TokenID:     ftID,
				DID:         did,
				TokenValue:  fractionalValue,
				TokenStatus: int16(constants.TokenStatus_Free),
				TokenType:   ftTokenTypeID,
				TransactionID: txID,
			}
			if err = c.w.PersistGenesisTokenRecord(genesisTx, t, &models.TokenChain{
				TokenID:               ftID,
				TransactionID:         txID,
				PreviousTransactionID: nil,
				Role:                  mintRoleID,
				Position:              0,
			}); err != nil {
				results <- ftResult{Err: fmt.Errorf("createFTs: failed to persist genesis token record for FT %s: %w", ftID, err)}
				continue
			}

			ft := wallet.FTToken{
				TokenID:     ftID,
				FTName:      FTName,
				TokenStatus: constants.TokenStatus_Free,
				TokenValue:  fractionalValue,
				DID:         did,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			results <- ftResult{FTToken: ft, FTID: ftID}

			// Publish the genesis transaction on the network with topic: rubix_txns
			publishingTxn := &model.PubSubTxnInfo{
				BlockHash:    txID,
				BlockType:    "05",
				AssetType:    FTTokenType,
				FTName:       FTName,
				PublisherDID: dc.GetDID(),
				CreatorDID:   dc.GetDID(),
			}

			err = c.publishTxn(publishingTxn)
			if err != nil {
				c.log.Error("Failed to publish txn", "err", err)
				results <- ftResult{Err: err}
				continue
			}
		}
	}

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker()
	}

	// Enqueue jobs
	for i := ftNumStartIndex; i < ftNumStartIndex+numFTs; i++ {
		jobs <- ftJob{Index: i}
	}
	close(jobs)

	// Collect results
	newFTs := make([]wallet.FTToken, 0, numFTs)
	var newFTTokenIDs []string
	var firstErr error
	for i := 0; i < numFTs; i++ {
		res := <-results
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
		}
		if res.FTID != "" {
			newFTTokenIDs = append(newFTTokenIDs, res.FTID)
		}
		if res.FTToken.TokenID != "" {
			newFTs = append(newFTs, res.FTToken)
		}
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	// newFTTokenIDs collected but not used after block package removal (was ChildTokens in old TokenChainBlock struct)
	_ = newFTTokenIDs

	// --- Burn parent RBT tokens via PersistPostConsensus ---
	burnTokenIDs := make([]string, 0, len(wholeTokens))
	burnTokenChainRows := make([]models.TokenChain, 0, len(wholeTokens))
	burnTokenStates := make([]models.Token, 0, len(wholeTokens))
	burnRoleID := int16(models.GetTokenRoleID(constants.TokenRole_Burn))

	// Collect parent token infos for the burn transaction info
	burnRBTTokenInfos := make([]*models.TokenInfo, 0, len(wholeTokens))
	for _, wt := range wholeTokens {
		burnRBTTokenInfos = append(burnRBTTokenInfos, &models.TokenInfo{
			TokenID: wt.TokenID,
		})
	}

	burnTxInfo := &models.TransactionInfo{
		Initiator: did,
		Owner:     did,
		Epoch:     int(currentTime.Unix()),
		Network:   constants.NetworkID_RBT_Testnet,
		Tokens: &models.TransactionTokens{
			RBT: burnRBTTokenInfos,
		},
	}

	burnInfoBytes, err := models.SerializeTransactionInfo(burnTxInfo)
	if err != nil {
		c.log.Error("Failed to serialize burn transaction info", "err", err)
		return fmt.Errorf("failed to serialize burn transaction info: %w", err)
	}
	burnSigBytes, err := dc.PvtSign(burnInfoBytes)
	if err != nil {
		c.log.Error("Failed to sign burn transaction", "err", err)
		return fmt.Errorf("failed to sign burn transaction: %w", err)
	}
	burnSignature := &models.Signature{InitiatorSignature: hex.EncodeToString(burnSigBytes)}
	burnTxID, err := wallet.ComputeTransactionID(burnTxInfo)
	if err != nil {
		c.log.Error("Failed to compute burn transaction ID", "err", err)
		return fmt.Errorf("failed to compute burn transaction ID: %w", err)
	}

	for _, wt := range wholeTokens {
		parentTokenRecord, err := c.w.ReadToken(wt.TokenID)
		if err != nil {
			c.log.Error("FT token creation failed, failed to read parent token record", "err", err)
			return err
		}
		burnTokenIDs = append(burnTokenIDs, wt.TokenID)
		prevTxID := parentTokenRecord.TransactionID
		burnTokenChainRows = append(burnTokenChainRows, models.TokenChain{
			TokenID:               wt.TokenID,
			TransactionID:         burnTxID,
			PreviousTransactionID: &prevTxID,
			Role:                  burnRoleID,
			Position:              parentTokenRecord.LatestPosition + 1,
		})
		burnTokenStates = append(burnTokenStates, models.Token{
			TokenID:        wt.TokenID,
			ParentTokenID:  parentTokenRecord.ParentTokenID,
			TokenValue:     parentTokenRecord.TokenValue,
			TokenStatus:    int16(constants.TokenStatus_BurntForFT),
			DID:            did,
			TransactionID:  burnTxID,
			TokenStateHash: parentTokenRecord.TokenStateHash,
			TokenType:      parentTokenRecord.TokenType,
			LatestPosition: parentTokenRecord.LatestPosition + 1,
			LatestRole:     burnRoleID,
		})
	}

	burnReq := &wallet.PostConsensusPersistenceRequest{
		TransactionInfo: burnTxInfo,
		Signature:       burnSignature,
		DID:             did,
		ExecutionRole:   wallet.ExecutionRoleInitiator,
		AffectedTokens:  burnTokenIDs,
		TokenChainRows:  burnTokenChainRows,
		TokenStates:     burnTokenStates,
	}
	if err := c.w.PersistPostConsensus(context.Background(), burnReq); err != nil {
		c.log.Error("Failed to persist parent RBT burn", "err", err)
		return fmt.Errorf("failed to persist parent RBT burn: %w", err)
	}
	c.log.Debug("parent RBT tokens burnt via PersistPostConsensus")

	// Publish burn events per parent token
	for _, wt := range wholeTokens {
		publishingTxn := &model.PubSubTxnInfo{
			BlockHash:    burnTxID,
			BlockType:    "07",
			AssetType:    RBTTokenType,
			PublisherDID: dc.GetDID(),
		}
		if pubErr := c.publishTxn(publishingTxn); pubErr != nil {
			c.log.Error("Failed to publish burn txn", "err", pubErr)
			// Non-fatal: burn is already persisted
			_ = wt
		}
	}

	// After all workers finish, batch add provider details
	err = c.w.AddProviderDetailsBatch(providerMaps)
	if err != nil {
		c.log.Error("Failed to batch add provider details for FTs", "err", err)
		return err
	}

	c.log.Debug("updating ft table with new fts")

	updateFTTableErr := c.updateFTTable()
	if updateFTTableErr != nil {
		c.log.Error("Failed to update FT table after FT creation", "err", updateFTTableErr)
		return updateFTTableErr
	}
	return nil
}

func (c *Core) GetFTInfoByDID(did string) ([]model.FTInfo, error) {
	if !c.w.IsDIDExist(did) {
		c.log.Error("DID does not exist")
		return nil, fmt.Errorf("DID does not exist")
	}
	FT, err := c.w.GetFTsAndCount(did)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens FTs and Count", "err", err)
		return []model.FTInfo{}, fmt.Errorf("Failed to get tokens FTs and Count")
	}
	ftInfoMap := make(map[string]map[string]int)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]int) // Initialize map for each FTName
		}
		ftInfoMap[t.FTName][t.CreatorDID] += t.FTCount // Increment count for the specific CreatorDID
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
	txEpoch := int(st.Unix())

	// Track overall FT transaction performance
	var txErr error
	defer func() {
		c.TrackOperation("tx.ft_transfer.total", map[string]interface{}{
			"sender":   req.Sender,
			"receiver": req.Receiver,
			"ft_count": req.FTCount,
			"ft_name":  req.FTName,
		})(txErr)
	}()

	resp := &model.BasicResponse{
		Status: false,
	}
	if req.Sender == req.Receiver {
		c.log.Error("Sender and receiver cannot same")
		resp.Message = "Sender and receiver cannot be same"
		txErr = fmt.Errorf("sender and receiver cannot be same")
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

	rpeerid, rdid, ok := util.ParseAddress(req.Receiver)
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
		// Checking for same FTs with different creators
		info, err := c.GetFTInfoByDID(did)
		if err != nil || info == nil {
			c.log.Error("Failed to get FT info for transfer", "err", err)
			resp.Message = "Failed to get FT info for transfer"
			return resp
		}
		ftNameToCreators := make(map[string][]string)
		for _, ft := range info {
			ftNameToCreators[ft.FTName] = append(ftNameToCreators[ft.FTName], ft.CreatorDID)
		}
		for ftName, creators := range ftNameToCreators {
			if len(creators) > 1 {
				c.log.Error(fmt.Sprintf("There are same FTs '%s' with different creators.", ftName))
				for i, creator := range creators {
					c.log.Error(fmt.Sprintf("Creator DID %d: %s", i+1, creator))
				}
				c.log.Info("Use -creatorDID flag to specify the creator DID and can proceed for transfer")
				resp.Message = "There are same FTs with different creators, use -creatorDID flag to specify creatorDID"
				return resp
			}
		}
		if info != nil && len(info) > 0 {
			creatorDID = info[0].CreatorDID
		}
	}
	// Get all available FT tokens
	var AllFTs []wallet.FTToken
	var TokenInfo []ContractTokenInfo

	if req.CreatorDID != "" {
		AllFTs, err = c.w.GetFreeFTsByNameAndCreatorDID(req.FTName, did, req.CreatorDID)
		creatorDID = req.CreatorDID
	} else {
		AllFTs, err = c.w.GetFreeFTsByNameAndDID(req.FTName, did)
	}

	AvailableFTCount := len(AllFTs)
	if err != nil {
		c.log.Error("Failed to get FTs", "err", err)
		resp.Message = "Insufficient FTs or FTs are locked or " + err.Error()
		return resp
	} else {
		if req.FTCount > AvailableFTCount {
			c.log.Error(fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount))
			resp.Message = fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
			return resp
		}
	}

	FTsForTxn := AllFTs[:req.FTCount]

	// Fetching peer's peer id
	if !c.w.IsDIDExist(req.Receiver) {
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
	}

	receiverPeerID, err := c.getPeer(req.Receiver)
	if err != nil {
		resp.Message = "Failed to get receiver peer, " + err.Error()
		return resp
	}
	defer receiverPeerID.Close()

	TokenInfo = make([]ContractTokenInfo, 0)
	for i := range FTsForTxn {
		lockFTErr := c.w.LockTokenByID(FTsForTxn[i].TokenID)
		if lockFTErr != nil {
			c.log.Error("Failed to update FT token status", "err", lockFTErr)
			resp.Message = "Failed to update FT token status"
			return resp
		}
		tt := c.TokenType(FTString)
		// Replace block-based GetLatestTokenBlock/GetBlockID with PostgreSQL token read
		tokenRecord, err := c.w.ReadToken(FTsForTxn[i].TokenID)
		if err != nil {
			c.log.Error("failed to read token record", "err", err)
			resp.Message = "failed to read token record, " + err.Error()
			return resp
		}
		ti := ContractTokenInfo{
			Token:      FTsForTxn[i].TokenID,
			TokenType:  tt,
			TokenValue: FTsForTxn[i].TokenValue,
			OwnerDID:   did,
			BlockID:    tokenRecord.TransactionID, // PostgreSQL transaction ID replaces block ID
		}
		TokenInfo = append(TokenInfo, ti)
	}

	// Extract token IDs for later use (reserved for future explorer/audit use)
	FTTokenIDs := make([]string, 0)

	for i := range TokenInfo {
		FTTokenIDs = append(FTTokenIDs, TokenInfo[i].Token)
	}
	_ = FTTokenIDs // collected for future use; currently unused after block removal

	sct := &ContractTypeInfo{
		Type:       SCFTType,
		PledgeMode: PeriodicPledgeMode,
		TransInfo: &ContractTransInfo{
			SenderDID:   did,
			ReceiverDID: rdid,
			Comment:     req.Comment,
			TransTokens: TokenInfo,
		},
		ReqID: reqID,
	}
	FTData := model.FTInfo{
		FTName:  req.FTName,
		FTCount: req.FTCount,
	}
	sc := CreateNewConsensusContract(sct)
	err = sc.UpdateSignature(dc)
	if err != nil {
		c.log.Error(err.Error())
		resp.Message = err.Error()
		return resp
	}
	cr := &ConensusRequest{
		Mode:             FTTransferMode,
		ReqID:            uuid.New().String(),
		Type:             req.QuorumType,
		SenderPeerID:     c.peerID,
		ReceiverPeerID:   rpeerid,
		ContractBlock:    sc.GetBlock(),
		FTinfo:           FTData,
		TransactionEpoch: txEpoch,
		OperationType:    req.OperationType,
	}

	resultChan := make(chan *model.BasicResponse, 1)

	// start transaction in go routine
	go func() {
		td, _, _, FTconsErr := c.initiateConsensus(cr, sc, dc)
		if FTconsErr != nil {
			resp.Message = fmt.Sprintf("Consensus failed: %s", FTconsErr.Error())
			tokens := sc.GetTransTokenInfo()
			for _, token := range tokens {
				if token.Token == "" {
					continue
				}

				readToken, ReadFTErr := c.w.ReadToken(token.Token)
				if ReadFTErr != nil {
					c.log.Error("Failed to read FT token", "token", token.Token, "err", ReadFTErr)
					resp.Message = "Failed to read FT token"
					resp.Status = false
					resultChan <- resp
					return
				}

				if readToken.TokenStatus == int16(constants.TokenStatus_Locked) {
					updateFTErr := c.w.UpdateToken(&models.Token{TokenID: token.Token, TokenStatus: int16(constants.TokenStatus_Free)})
					if updateFTErr != nil {
						c.log.Error("Failed to update FT token status", "token", token.Token, "err", updateFTErr)
						resp.Message = "Failed to update FT token status"
						resp.Status = false
						resultChan <- resp
						return
					}
				}
			}

			resp.Status = false
			resultChan <- resp
			return
		}
		et := time.Now()
		dif := et.Sub(st)
		td.Amount = float64(req.FTCount)
		td.TotalTime = float64(dif.Milliseconds())
		if td.TotalTime < 0.00 {
			td.TotalTime = 0.00
		}
		if err := c.w.AddTransactionHistory(td); err != nil {
			errMsg := fmt.Sprintf("Error occured while adding FT transaction details: %v", err)
			c.log.Error(errMsg)
			resp.Message = errMsg
			return
		}

		// Store in new FT transaction history table
		if err := c.w.AddFTTransactionHistory(td, req.FTName, creatorDID, req.FTCount); err != nil {
			c.log.Error("Failed to store FT transaction history", "err", err)
			// Don't fail the transaction, just log the error
		}

		// Store FT token metadata for sent transactions
		if err := c.w.AddFTTransactionTokens(td.TransactionID, creatorDID, req.FTName, req.FTCount, "sent"); err != nil {
			c.log.Error("Failed to store FT transaction token metadata", "err", err)
			// Don't fail the transaction, just log the error
		}

		// Create a channel to signal explorer submission completion
		explorerDone := make(chan struct{})

		go func() {
			defer close(explorerDone) // Signal completion when done

			allTokens := make([]AllToken, len(TokenInfo))
			for i := range TokenInfo {
				tokenDetail := AllToken{}
				tokenDetail.TokenHash = TokenInfo[i].Token

				// TODO(phase09): BlockID is now a transaction ID; update explorer token detail format
				blockNoPart := strings.Split(TokenInfo[i].BlockID, "-")[0]
				// Convert the string part to an int
				blockNoInt, err := strconv.Atoi(blockNoPart)
				if err != nil {
					log.Printf("Error getting BlockID: %v", err)
					continue
				}
				tokenDetail.BlockNumber = blockNoInt
				tokenDetail.BlockHash = strings.Split(TokenInfo[i].BlockID, "-")[1]

				allTokens[i] = tokenDetail
			}
			c.log.Info("Explorer submission completed", "transaction_id", td.TransactionID)
		}()

		// Pass the explorerDone channel to consensus request
		cr.ExplorerDone = explorerDone

		c.log.Info("FT Transfer finished successfully", "duration", dif, " trnxid", td.TransactionID)
		msg := fmt.Sprintf("FT Transfer finished successfully in %v with trnxid %v", dif, td.TransactionID)
		resp.Status = true
		resp.Message = msg
		if strings.Contains(resp.Message, "with transaction id") {
			if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
				resp.Result = txID
			}
		}

		updateFTTableErr := c.updateFTTable()
		if updateFTTableErr != nil {
			c.log.Error("Failed to update FT table after transfer ", "err", updateFTTableErr)
			resp.Message = "Failed to update FT table after transfer"
			return
		}

		// Send final transaction completion response if not already timed out
		select {
		case resultChan <- resp:
			// Successfully sent to resultChan
		default:
			// If no one is listening (already timed out), just log and exit
			c.log.Debug("FT Transaction completed but resultChan is not being read anymore")
		}

	}()

	if c.IsAsyncFTResponse() {
		select {
		case result := <-resultChan:
			// Transaction completed within 20s or failed
			c.log.Debug("FT transaction completed before 20 secs")
			return result
		case <-time.After(20 * time.Second):
			// Timeout occurred, return Transaction ID only
			c.log.Debug("FT transaction still processing with txn id ", cr.TransactionID)
			msg := fmt.Sprintf("FT Transaction is still processing, with transaction id %v ", cr.TransactionID)
			resp.Message = msg
			if strings.Contains(resp.Message, "with transaction id") {
				if txID := extractTransactionIDFromMessage(resp.Message); txID != "" {
					resp.Result = txID
				}
			}
			resp.Status = true
			return resp
		}
	} else {
		// Wait for the transaction to complete, no timeout
		result := <-resultChan
		return result
	}
}

func extractTransactionIDFromMessage(msg string) string {
	re := regexp.MustCompile(`[a-fA-F0-9]{64}`)
	return re.FindString(msg)
}

func (c *Core) GetPresiceFractionalValue(a, b int) (float64, error) {
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

func (c *Core) updateFTTable() error {
	// TODO(phase09): implement via PostgreSQL
	return nil
}

func (c *Core) UnlockFTs() error {
	lockedFTs, err := c.w.GetLockedFTs()
	if err != nil {
		c.log.Error("Failed to get locked FTs", "err", err)
		return err
	}

	for _, ft := range lockedFTs {
		if ft.TokenID == "" {
			continue
		}

		err := c.w.UpdateToken(&models.Token{TokenID: ft.TokenID, TokenStatus: int16(constants.TokenStatus_Free)})
		if err != nil {
			c.log.Error("Failed to unlock FT", "token_id", ft.TokenID, "err", err)
			continue
		}
	}
	c.log.Info("Unlocked FT")
	return nil
}

// Helper to check config flag
func (c *Core) IsAsyncFTResponse() bool {
	// Return the actual config value
	return c.cfg.CfgData.AsyncFTResponse
}

// FixAllFTTokensWithPeerIDAsCreator fixes all FT tokens that have peer ID as CreatorDID
func (c *Core) FixAllFTTokensWithPeerIDAsCreator() ([]wallet.FTTokenFixResult, error) {
	return c.w.FixAllFTTokensWithPeerIDAsCreator()
}

// GetFTTokenCreatorStats returns statistics about FT token creators
func (c *Core) GetFTTokenCreatorStats() (map[string]interface{}, error) {
	return c.w.GetFTTokenCreatorStats()
}
