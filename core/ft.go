package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"runtime"
	"sync"
	"sync/atomic"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, ftValue float64, ftNumStartIndex int, fromRBT bool, isHighValueFt bool) {
	err := c.createFTs(reqID, ftname, ftcount, ftValue, did, ftNumStartIndex, fromRBT, isHighValueFt)
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

func (c *Core) createFTs(reqID string, FTName string, numFTs int, FTValue float64, did string, ftNumStartIndex int, fromRBT bool, isHighValueFt bool) error {
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

	var ftStartIndex int
	ftStartIndex = ftNumStartIndex

	// Validate input parameters

	switch {
	case numFTs <= 0:
		return fmt.Errorf("number of tokens to create must be greater than zero")
	case FTValue < 0.001:
		return fmt.Errorf("FT value must be at least 0.001")
	case math.Round(FTValue*1000) != FTValue*1000:
		return fmt.Errorf("FT value cannot have more than 3 decimal places")
	}

	var wholeTokens []wallet.Token
	var parentTokenIDs string
	var parentTokenIDsArray []string

	if fromRBT && !isHighValueFt {
		numTokensRequired := FTValue * float64(numFTs)
		numWholeTokens := floatPrecision(numTokensRequired, MaxDecimalPlaces)

		wholeTokens, err = c.GetTokens(dc, did, numWholeTokens, 0)
		if err != nil {
			c.log.Error("Failed to fetch whole token for FT creation, RBTs required are: %v", numWholeTokens)
			return err
		}
		if wholeTokens == nil {
			return fmt.Errorf("no tokens available for FT creation")
		}
		defer c.w.ReleaseTokens(wholeTokens)

		for _, token := range wholeTokens {
			parentTokenIDsArray = append(parentTokenIDsArray, token.TokenID)
		}
		parentTokenIDs = strings.Join(parentTokenIDsArray, ",")
	}

	loopCount := numFTs
	if isHighValueFt {
		loopCount = 1
	}

	type ftJob struct {
		Index int
	}
	type ftResult struct {
		FTToken wallet.FTToken
		FTID    string
		Err     error
	}

	numWorkers := runtime.NumCPU()
	jobs := make(chan ftJob, loopCount)
	results := make(chan ftResult, loopCount)
	var wg sync.WaitGroup

	var completed int32
	var lastLoggedPercent int32

	// Prepare to collect provider details for batch write
	providerMaps := make([]model.TokenProviderMap, 0, numFTs)
	// Mutex for providerMaps slice
	var providerMapMutex sync.Mutex
	c.log.Info("Initializing FT creation: progress logging")

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			i := job.Index
			racType := &rac.RacType{
				Type:        c.RACFTType(),
				DID:         did,
				TokenNumber: uint64(i),
				TotalSupply: 1,
				TimeStamp:   time.Now().String(),
				FTInfo: &rac.RacFTInfo{
					Parents: parentTokenIDs,
					FTNum:   i,
					FTName:  FTName,
					FTValue: FTValue,
				},
			}

			// Create the RAC block
			racBlocks, err := rac.CreateRac(racType)
			if err != nil {
				results <- ftResult{Err: err}
				continue
			}
			if len(racBlocks) != 1 {
				results <- ftResult{Err: fmt.Errorf("failed to create RAC block")}
				continue
			}
			err = racBlocks[0].UpdateSignature(dc)
			if err != nil {
				results <- ftResult{Err: err}
				continue
			}

			ftnumString := strconv.Itoa(i)
			parts := []string{FTName, ftnumString, did}
			result := strings.Join(parts, " ")
			byteArray := []byte(result)
			ftBuffer := bytes.NewBuffer(byteArray)
			ftID, tpm, err := c.w.AddWithProviderMap(ftBuffer, did, wallet.AddFunc)
			if err != nil {
				results <- ftResult{Err: err}
				continue
			}
			// Collect provider map for batch
			// Use mutex to avoid race condition
			providerMapMutex.Lock()
			providerMaps = append(providerMaps, tpm)
			providerMapMutex.Unlock()
			// Progress logging
			newCount := atomic.AddInt32(&completed, 1)
			if isHighValueFt {
				c.log.Info(fmt.Sprintf("Creating high-value FT: %s", FTName))
			} else {
				currentPercent := int32(math.Floor(float64(newCount*100) / float64(loopCount)))
				if currentPercent%10 == 0 && atomic.LoadInt32(&lastLoggedPercent) < currentPercent {
					oldPercent := atomic.LoadInt32(&lastLoggedPercent)
					if atomic.CompareAndSwapInt32(&lastLoggedPercent, oldPercent, currentPercent) {
						c.log.Info(fmt.Sprintf("FT creation progress: %d%% (%d/%d created)", currentPercent, newCount, loopCount))
					}
				}
			}

			RBTLockStatus := block.RBTNotLocked
			if fromRBT && !isHighValueFt {
				RBTLockStatus = block.RBTLocked
			}

			bti := &block.TransInfo{
				Tokens: []block.TransTokens{{
					Token:     ftID,
					TokenType: c.TokenType(FTString),
				}},
				Comment: "FT generated at : " + time.Now().String() + " for FT Name : " + FTName,
			}
			tcb := &block.TokenChainBlock{
				TransactionType: block.TokenGeneratedType,
				TokenOwner:      did,
				TransInfo:       bti,
				GenesisBlock: &block.GenesisBlock{
					Info: []block.GenesisTokenInfo{{
						Token:         ftID,
						ParentID:      parentTokenIDs,
						TokenNumber:   i,
						RBTLockStatus: RBTLockStatus,
					}},
				},
				TokenValue: FTValue,
			}
			ctcb := make(map[string]*block.Block)
			ctcb[ftID] = nil
			blockObj := block.CreateNewBlock(ctcb, tcb)
			if blockObj == nil {
				results <- ftResult{Err: fmt.Errorf("failed to create new block")}
				continue
			}
			err = blockObj.UpdateSignature(dc)
			if err != nil {
				results <- ftResult{Err: err}
				continue
			}
			err = c.w.AddTokenBlock(ftID, blockObj)
			if err != nil {
				results <- ftResult{Err: err}
				continue
			}
			ft := wallet.FTToken{
				TokenID:       ftID,
				FTName:        FTName,
				TokenStatus:   wallet.TokenIsFree,
				TokenValue:    FTValue,
				DID:           did,
				RBTLockStatus: RBTLockStatus,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			results <- ftResult{FTToken: ft, FTID: ftID}
		}
	}

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker()
	}

	// Enqueue jobs
	for i := ftStartIndex; i < ftStartIndex+loopCount; i++ {
		jobs <- ftJob{Index: i}
	}
	close(jobs)

	// Collect results
	newFTs := make([]wallet.FTToken, 0, loopCount)
	var newFTTokenIDs []string
	var firstErr error
	for i := 0; i < loopCount; i++ {
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

	if fromRBT && !isHighValueFt {
		for i := range wholeTokens {
			ptts := RBTString
			if wholeTokens[i].ParentTokenID != "" && wholeTokens[i].TokenValue < 1 {
				ptts = PartString
			}
			ptt := c.TokenType(ptts)

			bti := &block.TransInfo{
				Tokens: []block.TransTokens{
					{
						Token:     wholeTokens[i].TokenID,
						TokenType: ptt,
					},
				},
				Comment: "Token locked for FT at : " + time.Now().String(),
			}

			tcb := &block.TokenChainBlock{
				TransactionType: block.TokenLockedForFT,
				TokenOwner:      did,
				TransInfo:       bti,
				TokenValue:      wholeTokens[i].TokenValue,
				ChildTokens:     newFTTokenIDs,
			}

			ctcb := make(map[string]*block.Block)
			ctcb[wholeTokens[i].TokenID] = c.w.GetLatestTokenBlock(wholeTokens[i].TokenID, ptt)

			blk := block.CreateNewBlock(ctcb, tcb)
			if blk == nil {
				return fmt.Errorf("failed to create new block")
			}

			err = blk.UpdateSignature(dc)
			if err != nil {
				c.log.Error("FT creation failed, failed to update signature", "err", err)
				return err
			}

			err = c.w.AddTokenBlock(wholeTokens[i].TokenID, blk)
			if err != nil {
				c.log.Error("FT creation failed, failed to add token block", "err", err)
				return err
			}

			// Mark the token as locked for FT
			wholeTokens[i].TokenStatus = wallet.TokenIsLockedForFT
			err = c.w.UpdateToken(&wholeTokens[i])
			if err != nil {
				c.log.Error("FT token creation failed, failed to update token status", "err", err)
				return err
			}
		}
	}

	// --- Batch Write FTs to Storage using WriteBatch ---
	var batch []*wallet.FTToken
	for i := range newFTs {
		if newFTs[i].DID == did {
			newFTs[i].CreatorDID = did
		} else {
			tt := c.TokenType(FTString)
			blk := c.w.GetGenesisTokenBlock(newFTs[i].TokenID, tt)
			if blk == nil {
				c.log.Error("failed to get genesis block for Parent DID updation, invalid token chain")
				return fmt.Errorf("failed to get genesis block for Parent DID updation, invalid token chain")
			}
			FTOwner := blk.GetOwner()
			newFTs[i].CreatorDID = FTOwner
		}
		batch = append(batch, &newFTs[i])
	}
	batchSize := 1000 // or tune as needed
	// 1. Write to SQL DB first
	err = c.w.S().WriteBatch(wallet.FTTokenStorage, batch, batchSize)
	if err != nil {
		c.log.Error("Failed to batch write FT tokens (SQL phase)", "err", err)
		return err
	}

	// 2. Write all token chain blocks to LevelDB in a batch
	var blockPairs []struct {
		Token string
		Block *block.Block
	}
	for i := range newFTs {
		ft := &newFTs[i]
		blockObj := c.w.GetLatestTokenBlock(ft.TokenID, c.TokenType(FTString))
		if blockObj == nil {
			c.log.Error("Failed to get latest token block for FT", "token_id", ft.TokenID)
			// Rollback SQL writes
			for _, rollbackFT := range newFTs {
				errDel := c.w.S().Delete(wallet.FTTokenStorage, &rollbackFT, "token_id=?", rollbackFT.TokenID)
				if errDel != nil {
					c.log.Error("Rollback failed: could not delete FT from SQL after LevelDB failure", "token_id", rollbackFT.TokenID, "err", errDel)
				}
			}
			return fmt.Errorf("failed to get latest token block for FT %s", ft.TokenID)
		}
		blockPairs = append(blockPairs, struct {
			Token string
			Block *block.Block
		}{Token: ft.TokenID, Block: blockObj})
	}
	if err := c.w.BatchAddTokenBlocksFT(blockPairs); err != nil {
		c.log.Error("Failed to batch add token blocks to LevelDB after SQL write", "err", err)
		// Rollback SQL writes
		for _, rollbackFT := range newFTs {
			errDel := c.w.S().Delete(wallet.FTTokenStorage, &rollbackFT, "token_id=?", rollbackFT.TokenID)
			if errDel != nil {
				c.log.Error("Rollback failed: could not delete FT from SQL after LevelDB failure", "token_id", rollbackFT.TokenID, "err", errDel)
			}
		}
		return fmt.Errorf("failed to batch add token blocks to LevelDB: %v", err)
	}

	// After all workers finish, batch add provider details
	err = c.w.AddProviderDetailsBatch(providerMaps)
	if err != nil {
		c.log.Error("Failed to batch add provider details for FTs", "err", err)
		return err
	}

	// Refresh FTTable using full recompute to ensure IDs are populated
	// Here the main issue is that, the ID in this table is string, which ideally should be integer. inorder to maintain backward compatibility we had to use this function.
	// This needs to be updated such that the existing table will get migrated or some work around needs to be done.
	if err := c.updateFTTable(); err != nil {
		c.log.Error("Failed to update FT table after FT creation", "err", err)
		return err
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
		return []model.FTInfo{}, fmt.Errorf("failed to get tokens FTs and Count")
	}
	// Build map keyed by ft_name -> creator_did with count and total value
	type FTBalanceSummary struct {
		Count int
		Total float64
		High  bool
	}
	ftInfoMap := make(map[string]map[string]*FTBalanceSummary)

	// Iterate through retrieved FTs and populate the map
	for _, t := range FT {
		if ftInfoMap[t.FTName] == nil {
			ftInfoMap[t.FTName] = make(map[string]*FTBalanceSummary)
		}
		if ftInfoMap[t.FTName][t.CreatorDID] == nil {
			ftInfoMap[t.FTName][t.CreatorDID] = &FTBalanceSummary{}
		}
		// Count is already aggregated by wallet layer; track it
		ftInfoMap[t.FTName][t.CreatorDID].Count += t.FTCount
		// Determine if this FT is high value by looking up FT table row
		var meta wallet.FT
		if readErr := c.s.Read(wallet.FTStorage, &meta, "ft_name=? AND creator_did=?", t.FTName, t.CreatorDID); readErr == nil {
			ftInfoMap[t.FTName][t.CreatorDID].High = meta.HighValueFT
		}
		// If HVFT, compute total by summing token values of free tokens for this did+name
		// We'll compute totals after the loop to avoid repeated queries
	}
	info := make([]model.FTInfo, 0)
	for ftName, creators := range ftInfoMap {
		for creatorDID, balanceSummary := range creators {
			totalValue := 0.0
			if balanceSummary.High {
				// Sum token_value of free tokens for this DID+name
				var tokens []wallet.FTToken
				if err := c.s.Read(wallet.FTTokenStorage, &tokens, "owner_did=? AND token_status=? AND ft_name=?", did, wallet.TokenIsFree, ftName); err == nil {
					for _, tok := range tokens {
						if tok.CreatorDID == creatorDID {
							totalValue += tok.TokenValue
						}
					}
				}
			}
			info = append(info, model.FTInfo{
				FTName:       ftName,
				FTCount:      balanceSummary.Count,
				CreatorDID:   creatorDID,
				HighValueFT:  balanceSummary.High,
				FTTotalValue: totalValue,
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
	var TokenInfo []contract.TokenInfo
	var lockingErr error

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
	}
	if req.FTCount > AvailableFTCount {
		c.log.Error(fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount))
		resp.Message = fmt.Sprint("Insufficient balance, Available FT balance is ", AvailableFTCount, " trnx value is ", req.FTCount)
		return resp
	}

	// Select tokens based on high-value FT mode or standard count-based mode
	var FTsForTxn []wallet.FTToken
	if req.IsHighValueFT {
		// High-value FT mode: select tokens by value
		if req.FTTransferValue <= 0.0 {
			c.log.Error("FT transfer value must be greater than 0 for high-value FT")
			resp.Message = "FT transfer value is required for high-value FT transfer"
			return resp
		}
		requiredValue := floatPrecision(req.FTTransferValue, MaxDecimalPlaces)

		FTsForTxn, err = c.GetClosestTokens(dc, did, requiredValue, req.FTName)
		if err != nil {
			resp.Message = "Failed to select tokens for high-value FT: " + err.Error()
			return resp
		}
		c.log.Info("Selected FTs for high-value transfer", "count", len(FTsForTxn), "total_value", requiredValue)
	} else {
		// Standard FT mode: select tokens by count
		FTsForTxn = AllFTs[:req.FTCount]
	}

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

	// Use optimized locking for transfers > 100 tokens
	if c.shouldUseOptimizedFTLocking(req.FTCount) {
		c.log.Info("Using optimized FT locking", "ft_count", req.FTCount)
		TokenInfo, lockingErr = c.OptimizedFTTransferLocking(FTsForTxn, did, req.FTCount)
		if lockingErr != nil {
			c.log.Error("Failed to lock FT tokens optimized", "err", lockingErr)
			resp.Message = "Failed to lock FT tokens: " + lockingErr.Error()
			return resp
		}
	} else {
		// Original logic for small transfers
		TokenInfo = make([]contract.TokenInfo, 0)
		for i := range FTsForTxn {
			FTsForTxn[i].TokenStatus = wallet.TokenIsLocked
			lockFTErr := c.s.Update(wallet.FTTokenStorage, &FTsForTxn[i], "token_id=?", FTsForTxn[i].TokenID)
			if lockFTErr != nil {
				c.log.Error("Failed to update FT token status", "err", lockFTErr)
				resp.Message = "Failed to update FT token status"
				return resp
			}
			tt := c.TokenType(FTString)
			blk := c.w.GetLatestTokenBlock(FTsForTxn[i].TokenID, tt)
			if blk == nil {
				c.log.Error("failed to get latest block, invalid token chain")
				resp.Message = "failed to get latest block, invalid token chain"
				return resp
			}
			bid, err := blk.GetBlockID(FTsForTxn[i].TokenID)
			if err != nil {
				c.log.Error("failed to get block id", "err", err)
				resp.Message = "failed to get block id, " + err.Error()
				return resp
			}
			ti := contract.TokenInfo{
				Token:      FTsForTxn[i].TokenID,
				TokenType:  tt,
				TokenValue: FTsForTxn[i].TokenValue,
				OwnerDID:   did,
				BlockID:    bid,
			}
			TokenInfo = append(TokenInfo, ti)
		}
	}

	// Extract token IDs for later use
	FTTokenIDs := make([]string, 0)
	for i := range TokenInfo {
		FTTokenIDs = append(FTTokenIDs, TokenInfo[i].Token)
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
		FTName:      req.FTName,
		FTCount:     req.FTCount,
		HighValueFT: req.IsHighValueFT,
	}
	sc := contract.CreateNewContract(sct)
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
	}

	resultChan := make(chan *model.BasicResponse, 1)

	// start transaction in go routine
	go func() {
		td, _, pds, FTconsErr := c.initiateConsensus(cr, sc, dc)
		if FTconsErr != nil {
			resp.Message = fmt.Sprintf("Consensus failed: %s", FTconsErr.Error())
			tokens := sc.GetTransTokenInfo()
			for _, token := range tokens {
				if token.Token == "" {
					continue
				}

				ftToken := &wallet.FTToken{}
				ReadFTErr := c.s.Read(wallet.FTTokenStorage, ftToken, "token_id=?", token.Token)
				if ReadFTErr != nil {
					c.log.Error("Failed to read FT token", "token", token.Token, "err", ReadFTErr)
					resp.Message = "Failed to read FT token"
					resp.Status = false
					resultChan <- resp
					return
				}

				if ftToken.TokenStatus == wallet.TokenIsLocked {
					ftToken.TokenStatus = wallet.TokenIsFree
					updateFTErr := c.s.Update(wallet.FTTokenStorage, ftToken, "token_id=?", token.Token)
					if updateFTErr != nil {
						c.log.Error("Failed to update FT token status", "token", token.Token, "err", updateFTErr)
						resp.Message = "Failed to update FT token status"
						resp.Status = false
						resultChan <- resp
						return
					}
				}
			}
			c.UpdateUserInfo([]string{did})
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

			AllTokens := make([]AllToken, len(TokenInfo))
			for i := range TokenInfo {
				tokenDetail := AllToken{}
				tokenDetail.TokenHash = TokenInfo[i].Token

				blockNoPart := strings.Split(TokenInfo[i].BlockID, "-")[0]
				// Convert the string part to an int
				blockNoInt, err := strconv.Atoi(blockNoPart)
				if err != nil {
					log.Printf("Error getting BlockID: %v", err)
					continue
				}
				tokenDetail.BlockNumber = blockNoInt
				tokenDetail.BlockHash = strings.Split(TokenInfo[i].BlockID, "-")[1]

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
				FTSymbol:        "N/A",
				Comments:        req.Comment,
				TransactionID:   td.TransactionID,
				PledgeInfo:      PledgeInfo{PledgeDetails: pds.PledgedTokens, PledgedTokenList: pds.TokenList},
				QuorumList:      extractQuorumDID(cr.QuorumList),
				Amount:          TokenInfo[0].TokenValue * float64(req.FTCount),
				FTTokenList:     FTTokenIDs,
			}
			c.ec.ExplorerFTTransaction(eTrans)
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
		c.UpdateUserInfo([]string{did})
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

func (c *Core) UpsertFTTable(ftName string, creatorDid string, _ int, numFTsCreatedInThisCall int, isHighValue bool) error {
	var existingFt wallet.FT

	err := c.s.Read(wallet.FTStorage, &existingFt, "ft_name=? AND creator_did=?", ftName, creatorDid)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "no records found") {
			newFT := &wallet.FT{
				FTName:      ftName,
				CreatorDID:  creatorDid,
				FTCount:     numFTsCreatedInThisCall,
				HighValueFT: isHighValue,
			}
			if writeErr := c.s.Write(wallet.FTStorage, newFT); writeErr != nil {
				return fmt.Errorf("failed to insert new record: %w", writeErr)
			}
			c.log.Info("New FT record created")
			return nil
		}
		return fmt.Errorf("error checking for existing record: %w", err)
	}

	existingFt.FTCount += numFTsCreatedInThisCall
	existingFt.HighValueFT = isHighValue

	updateErr := c.s.Update(wallet.FTStorage, &existingFt, "ft_name=? AND creator_did=?", ftName, creatorDid)
	if updateErr != nil {
		return fmt.Errorf("failed to update FT index: %w", updateErr)
	}
	return nil
}

func (c *Core) updateFTTable() error {
	AllFTs, err := c.w.GetAllFTsAndCount()
	// If no records are found, remove all entries from the FT table
	if err != nil {
		fetchErr := fmt.Sprint(err)
		if strings.Contains(fetchErr, "no records found") {
			err = c.s.Delete(wallet.FTStorage, &wallet.FT{}, "ft_name!=?", "")
			if err != nil {
				deleteErr := fmt.Sprint(err)
				if strings.Contains(deleteErr, "no records found") {
					c.log.Info("FT table is empty")
				} else {
					c.log.Error("Failed to delete all entries from FT table:", err)
					return err
				}
			}
			return nil
		} else {
			c.log.Error("Failed to get FTs and Count")
			return err
		}
	}
	err = c.s.Delete(wallet.FTStorage, &wallet.FT{}, "ft_name!=?", "")
	ReadErr := fmt.Sprint(err)
	if err != nil {
		if strings.Contains(ReadErr, "no records found") {
			c.log.Info("FT table is empty")
		}
		c.log.Error("Failed to remove current FTs from storage to add new:", err)
		return err
	}
	for _, Ft := range AllFTs {
		addErr := c.s.Write(wallet.FTStorage, &Ft)
		if addErr != nil {
			c.log.Error("Failed to add new FT:", Ft.FTName, "Error:", addErr)
			return addErr
		}
	}
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

		ft.TokenStatus = wallet.TokenIsFree

		// First, delete the token
		err := c.s.Delete(wallet.FTTokenStorage, &wallet.FT{}, "token_id=?", ft.TokenID)
		if err != nil {
			c.log.Error("Failed to delete FT", "token_id", ft.TokenID, "err", err)
			continue
		}

		// Then, re-insert the same token — this moves it to the bottom (new rowid)
		err = c.s.Write(wallet.FTTokenStorage, &ft)
		if err != nil {
			c.log.Error("Failed to re-insert FT", "token_id", ft.TokenID, "err", err)
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

// findBestCombination finds the optimal combination of tokens that sum to the target value
// Uses bit manipulation to generate all possible combinations (2^n - 1)
// Returns: exact match, or best over-match, or best under-match
func (c *Core) findBestCombination(tokens []wallet.FTToken, targetValue float64) ([]wallet.FTToken, float64, error) {
	c.log.Warn("Finding best combination of tokens", "targetValue", targetValue, "availableTokens", len(tokens))
	var bestTokens []wallet.FTToken
	bestSum := 0.0
	n := len(tokens)

	minExcess := math.MaxFloat64
	var overMatchTokens []wallet.FTToken
	overMatchSum := 0.0

	for i := 1; i < (1 << n); i++ {
		var currentTokens []wallet.FTToken
		sum := 0.0
		for j := 0; j < n; j++ {
			if i&(1<<j) != 0 {
				currentTokens = append(currentTokens, tokens[j])
				sum += tokens[j].TokenValue
			}
		}

		if sum == targetValue {
			return currentTokens, sum, nil // Exact match
		}

		if sum < targetValue && sum > bestSum {
			bestTokens = make([]wallet.FTToken, len(currentTokens))
			copy(bestTokens, currentTokens)
			bestSum = sum
		}

		if sum > targetValue && (sum-targetValue) < minExcess {
			overMatchTokens = make([]wallet.FTToken, len(currentTokens))
			copy(overMatchTokens, currentTokens)
			overMatchSum = sum
			minExcess = sum - targetValue
		}
	}

	if len(overMatchTokens) > 0 {
		return overMatchTokens, overMatchSum, nil
	}

	if len(bestTokens) > 0 {
		return bestTokens, bestSum, nil
	}

	return nil, 0, fmt.Errorf("no valid combination found")
}

// findClosestToken finds and locks a single token with value closest to the target
func (c *Core) findClosestToken(ownerDID string, targetValue float64) ([]wallet.FTToken, error) {
	var tokens []wallet.FTToken
	err := c.s.Read(wallet.FTTokenStorage, &tokens, "owner_did=? AND token_status=?", ownerDID, wallet.TokenIsFree)
	if err != nil {
		c.log.Error("Failed to query all free tokens", "err", err)
		return nil, err
	}
	if len(tokens) == 0 {
		c.log.Error("No free tokens found for owner DID", "owner_did", ownerDID)
		return nil, fmt.Errorf("no free tokens found for owner DID %s", ownerDID)
	}

	// Find the token with value closest to targetValue
	var closestToken wallet.FTToken
	minDiff := int64(math.MaxInt64)
	for _, token := range tokens {
		diff := int64(math.Abs(float64(token.TokenValue - targetValue)))
		if diff < minDiff {
			minDiff = diff
			closestToken = token
		}
	}

	// Lock the closest token
	closestToken.TokenStatus = wallet.TokenIsLocked
	err = c.s.Update(wallet.FTTokenStorage, &closestToken, "owner_did=? AND token_id=?", ownerDID, closestToken.TokenID)
	if err != nil {
		c.log.Error("Failed to lock closest token", "err", err, "token_id", closestToken.TokenID)
		return nil, err
	}
	c.log.Debug("Selected closest token", "token_id", closestToken.TokenID, "value", closestToken.TokenValue, "targetValue", targetValue)
	return []wallet.FTToken{closestToken}, nil
}

// splitFTToken splits a single FT token into two new tokens (target value + balance)
// Burns the parent token and creates two child tokens
func (c *Core) splitFTToken(dc did.DIDCrypto, did, token string, currentTokenValue float64, newTokenValue float64) ([]wallet.FTToken, error) {
	if dc == nil {
		return nil, fmt.Errorf("did crypto is not initialised")
	}
	t, err := c.w.ReadFTToken(token)
	if err != nil || t == nil {
		return nil, fmt.Errorf("failed to get the specified token or the ft token does not exist")
	}
	if t.TokenStatus != wallet.TokenIsFree {
		return nil, fmt.Errorf("token is not free, current status: %d", t.TokenStatus)
	}
	release := true
	defer c.relaseToken(&release, token)
	tokenType := c.TokenType(FTString)

	parentBlockDetails := c.w.GetGenesisTokenBlock(token, tokenType)
	p, gp, err := parentBlockDetails.GetParentDetials(token)
	if gp == nil {
		gp = make([]string, 0)
	}
	if p != "" {
		gp = append(gp, p)
	}
	if err != nil {
		c.log.Error("failed to get parent details", "err", err)
		return nil, err
	}

	// Create RAC block for target token
	targetFT := &rac.RacType{
		Type:        c.RACFTType(),
		DID:         t.CreatorDID,
		TotalSupply: 1,
		TimeStamp:   time.Now().String(),
		FTInfo: &rac.RacFTInfo{
			Parents: token,
			FTNum:   1,
			FTName:  t.FTName,
			FTValue: newTokenValue,
		},
	}
	racBlocks, err := rac.CreateRac(targetFT)
	if err != nil {
		c.log.Error("Failed to create RAC block", "err", err)
		return nil, err
	}

	if len(racBlocks) != 1 {
		return nil, fmt.Errorf("failed to create RAC block")
	}

	err = racBlocks[0].UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update DID signature", "err", err)
		return nil, err
	}

	ftnumString := strconv.Itoa(1)
	parts := []string{t.FTName, ftnumString, did, time.Now().String()}
	result := strings.Join(parts, " ")
	byteArray := []byte(result)
	ftBuffer := bytes.NewBuffer(byteArray)
	ftID, err := c.w.Add(ftBuffer, did, wallet.AddFunc)
	if err != nil {
		c.log.Error("Failed to create FT, Failed to add token to IPFS", "err", err)
		return nil, err
	}
	c.log.Info("FT created: " + ftID + " FT Num: " + ftnumString)
	targetFtId := ftID
	c.log.Info("The target FT Id is :", targetFtId)

	RBTLockStatus := block.RBTNotLocked
	bti := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     ftID,
				TokenType: c.TokenType(FTString),
			},
		},
		Comment: "FT generated at : " + time.Now().String() + " for FT Name : " + t.FTName,
	}
	tcb := &block.TokenChainBlock{
		TransactionType: block.TokenGeneratedType,
		TokenOwner:      t.CreatorDID,
		TransInfo:       bti,
		GenesisBlock: &block.GenesisBlock{
			Info: []block.GenesisTokenInfo{
				{
					Token:         ftID,
					ParentID:      token,
					TokenNumber:   1,
					RBTLockStatus: RBTLockStatus,
				},
			},
		},
		TokenValue: newTokenValue,
	}
	ctcb := make(map[string]*block.Block)
	ctcb[ftID] = nil
	blk := block.CreateNewBlock(ctcb, tcb)
	if blk == nil {
		return nil, fmt.Errorf("failed to create new block")
	}
	err = blk.UpdateSignature(dc)
	if err != nil {
		c.log.Error("FT creation failed, failed to update signature", "err", err)
		return nil, err
	}
	err = c.w.AddTokenBlock(ftID, blk)
	if err != nil {
		c.log.Error("Failed to create FT, failed to add token chain block", "err", err)
		return nil, err
	}

	// Create the new target token
	targetFtInfo := wallet.FTToken{
		TokenID:       ftID,
		FTName:        t.FTName,
		TokenStatus:   wallet.TokenIsFree,
		TokenValue:    newTokenValue,
		DID:           did,
		RBTLockStatus: RBTLockStatus,
		CreatorDID:    t.CreatorDID,
	}

	err = c.w.CreateFT(&targetFtInfo)
	if err != nil {
		c.log.Error("Failed to write FT details in FT tokens table", "err", err)
		return nil, err
	}

	// Create balance token
	balanceValue := currentTokenValue - newTokenValue
	balanceFT := &rac.RacType{
		Type:        c.RACFTType(),
		DID:         t.CreatorDID,
		TotalSupply: 1,
		TimeStamp:   time.Now().String(),
		FTInfo: &rac.RacFTInfo{
			Parents: token,
			FTNum:   2,
			FTName:  t.FTName,
			FTValue: balanceValue,
		},
	}
	balanceFTRacBlocks, err := rac.CreateRac(balanceFT)
	if err != nil {
		c.log.Error("Failed to create RAC block", "err", err)
		return nil, err
	}

	if len(balanceFTRacBlocks) != 1 {
		return nil, fmt.Errorf("failed to create RAC block")
	}

	err = balanceFTRacBlocks[0].UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update DID signature", "err", err)
		return nil, err
	}

	ftnumString2 := strconv.Itoa(2)
	balanceFtdetailsAddedtoIpfs := []string{t.FTName, ftnumString2, did, time.Now().String()}
	result2 := strings.Join(balanceFtdetailsAddedtoIpfs, " ")
	byteArray2 := []byte(result2)
	ftBuffer2 := bytes.NewBuffer(byteArray2)
	balanceFtID, err := c.w.Add(ftBuffer2, did, wallet.AddFunc)
	if err != nil {
		c.log.Error("Failed to create FT, Failed to add token to IPFS", "err", err)
		return nil, err
	}
	c.log.Info("FT created: " + ftID + " FT Num: " + ftnumString)

	c.log.Info("The target FT Id is :", balanceFtID)

	bti2 := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     balanceFtID,
				TokenType: c.TokenType(FTString),
			},
		},
		Comment: "FT generated at : " + time.Now().String() + " for FT Name : " + t.FTName,
	}
	tcb2 := &block.TokenChainBlock{
		TransactionType: block.TokenGeneratedType,
		TokenOwner:      t.CreatorDID,
		TransInfo:       bti2,
		GenesisBlock: &block.GenesisBlock{
			Info: []block.GenesisTokenInfo{
				{
					Token:         balanceFtID,
					ParentID:      token,
					TokenNumber:   2,
					RBTLockStatus: RBTLockStatus,
				},
			},
		},
		TokenValue: balanceValue,
	}
	ctcb2 := make(map[string]*block.Block)
	ctcb2[balanceFtID] = nil
	blk2 := block.CreateNewBlock(ctcb2, tcb2)
	if blk2 == nil {
		return nil, fmt.Errorf("failed to create new block")
	}
	err = blk2.UpdateSignature(dc)
	if err != nil {
		c.log.Error("FT creation failed, failed to update signature", "err", err)
		return nil, err
	}
	err = c.w.AddTokenBlock(balanceFtID, blk2)
	if err != nil {
		c.log.Error("Failed to create FT, failed to add token chain block", "err", err)
		return nil, err
	}

	// Create the balance token
	balanceFTInfo := wallet.FTToken{
		TokenID:       balanceFtID,
		FTName:        t.FTName,
		TokenStatus:   wallet.TokenIsFree,
		TokenValue:    balanceValue,
		DID:           did,
		RBTLockStatus: RBTLockStatus,
		CreatorDID:    t.CreatorDID,
	}

	err = c.w.CreateFT(&balanceFTInfo)
	if err != nil {
		c.log.Error("Failed to write FT details in FT tokens table", "err", err)
		return nil, err
	}

	// Burn the parent token
	ftBurnBlock := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     token,
				TokenType: tokenType,
			},
		},
		Comment: "Token burnt at : " + time.Now().String(),
	}
	parentFtBlock := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       ftBurnBlock,
		TokenValue:      currentTokenValue,
		ChildTokens:     []string{targetFtId, balanceFtID},
	}
	ctcb3 := make(map[string]*block.Block)
	ctcb3[token] = c.w.GetLatestTokenBlock(token, tokenType)
	parentBlockDetails = block.CreateNewBlock(ctcb3, parentFtBlock)
	if parentBlockDetails == nil {
		return nil, fmt.Errorf("failed to create new block")
	}
	err = parentBlockDetails.UpdateSignature(dc)
	if err != nil {
		c.log.Error("part token creation failed, failed to update signature", "err", err)
		return nil, err
	}
	err = c.w.AddTokenBlock(token, parentBlockDetails)
	if err != nil {
		c.log.Error("part token creation failed, failed to add token block", "err", err)
		return nil, err
	}
	t.TokenStatus = wallet.TokenIsBurnt
	err = c.s.Update(wallet.FTTokenStorage, &t, "owner_did=? AND token_id=?", did, t.TokenID)
	if err != nil {
		c.log.Error("failed to update the token status to token is burned")
	}

	var ftTokens []wallet.FTToken
	ftTokens = append(ftTokens, balanceFTInfo, targetFtInfo)

	return ftTokens, nil
}

// GetClosestTokens finds the optimal combination of FT tokens to match a target value
// Implements multiple strategies: exact match, best combination, token splitting
func (c *Core) GetClosestTokens(dc did.DIDCrypto, ownerDID string, targetValue float64, FTName string) ([]wallet.FTToken, error) {
	c.log.Warn("GetClosestTokens called", "owner_did", ownerDID, "target_value", targetValue, "ft_name", FTName)
	var tokens []wallet.FTToken

	err := c.s.Read(wallet.FTTokenStorage, &tokens, "owner_did=? AND token_status=? AND ft_name=?", ownerDID, wallet.TokenIsFree, FTName)
	if err != nil {
		c.log.Error("Failed to query free tokens", "err", err)
		return nil, err
	}

	if len(tokens) == 0 {
		c.log.Warn("No tokens available")
		return nil, fmt.Errorf("insufficient balance")
	}

	// Step 1: Check total available balance
	total := 0.0
	for _, t := range tokens {
		total += t.TokenValue
	}
	if total < targetValue {
		c.log.Warn("Total balance is less than target", "total", total, "required", targetValue)
		return nil, fmt.Errorf("insufficient balance")
	}

	// Step 2: Check for exact match
	for _, token := range tokens {
		c.log.Warn("Checking for exact match", "token_value", token.TokenValue, "target_value", targetValue)
		if token.TokenValue == targetValue {
			token.TokenStatus = wallet.TokenIsLocked
			err := c.s.Update(wallet.FTTokenStorage, &token, "owner_did=? AND token_id=?", ownerDID, token.TokenID)
			if err != nil {
				return nil, err
			}
			return []wallet.FTToken{token}, nil
		}
	}

	// Step 3: Try to find exact combination
	selectedTokens, sum, err := c.findBestCombination(tokens, targetValue)
	c.log.Warn("Best combination found", "sum", sum, "target_value", targetValue)
	if err == nil && sum == targetValue {
		for i := range selectedTokens {
			selectedTokens[i].TokenStatus = wallet.TokenIsLocked
			err := c.s.Update(wallet.FTTokenStorage, &selectedTokens[i], "owner_did=? AND token_id=?", ownerDID, selectedTokens[i].TokenID)
			if err != nil {
				return nil, err
			}
		}
		return selectedTokens, nil
	}

	// Step 4: Try to split a token from an over-sum combination
	if err == nil && sum > targetValue {
		c.log.Warn("Trying to split a token from an over-sum combination", "sum", sum, "target_value", targetValue)
		excess := sum - targetValue
		c.log.Warn("Excess value to split", "excess", excess)
		for i, token := range selectedTokens {
			if token.TokenValue > excess {
				splitTokens, splitErr := c.splitFTToken(dc, ownerDID, token.TokenID, token.TokenValue, excess)
				if splitErr != nil {
					continue
				}

				var splitOutToken *wallet.FTToken
				desiredValue := token.TokenValue - excess
				for _, t := range splitTokens {
					if t.TokenValue == desiredValue {
						splitOutToken = &t
						break
					}
				}
				if splitOutToken == nil {
					continue
				}

				// Build final list excluding the burned token
				var finalTokens []wallet.FTToken
				for j, t := range selectedTokens {
					if j == i {
						continue // skip burned token
					}
					t.TokenStatus = wallet.TokenIsLocked
					err := c.s.Update(wallet.FTTokenStorage, &t, "owner_did=? AND token_id=?", ownerDID, t.TokenID)
					if err != nil {
						return nil, err
					}
					finalTokens = append(finalTokens, t)
				}
				c.log.Info("Split Token Found", "expected", desiredValue, "actual", splitOutToken.TokenValue)
				splitOutToken.TokenStatus = wallet.TokenIsLocked
				err := c.s.Update(wallet.FTTokenStorage, splitOutToken, "owner_did=? AND token_id=?", ownerDID, splitOutToken.TokenID)
				if err != nil {
					return nil, err
				}
				finalTokens = append(finalTokens, *splitOutToken)
				return finalTokens, nil
			}
		}
	}

	// Step 5: Try to find a single token > targetValue for split
	for _, token := range tokens {
		c.log.Warn("Checking for single token split", "token_value", token.TokenValue, "target_value", targetValue)
		if token.TokenValue > targetValue {
			splitTokens, splitErr := c.splitFTToken(dc, ownerDID, token.TokenID, token.TokenValue, targetValue)
			if splitErr != nil {
				continue
			}

			var splitOutToken *wallet.FTToken
			for _, t := range splitTokens {
				if t.TokenValue == targetValue {
					splitOutToken = &t
					break
				}
			}
			if splitOutToken == nil {
				continue
			}

			splitOutToken.TokenStatus = wallet.TokenIsLocked
			return []wallet.FTToken{*splitOutToken}, nil
		}
	}

	return nil, fmt.Errorf("unable to fulfill the request: no suitable token combination or split possible")
}

// findTokenOfValue finds and returns a token with the specified value from the slice
func findTokenOfValue(tokens []wallet.FTToken, value float64) wallet.FTToken {
	for _, t := range tokens {
		if t.TokenValue == value {
			return t
		}
	}
	return wallet.FTToken{} // Return zero value if not found
}

// burnSingleFT burns a single FT token and updates its status
func (c *Core) burnSingleFT(ft wallet.FTToken, did string, dc did.DIDCrypto) error {
	// Create FT burn info
	ftInfo := &block.TransInfo{
		Tokens: []block.TransTokens{
			{
				Token:     ft.TokenID,
				TokenType: c.TokenType(FTString),
			},
		},
		Comment: "FT burnt at : " + time.Now().String(),
	}

	// Create burn block
	ftBurntBlock := &block.TokenChainBlock{
		TransactionType: block.TokenBurntType,
		TokenOwner:      did,
		TransInfo:       ftInfo,
		TokenValue:      ft.TokenValue,
	}

	ftBurntBlockMap := make(map[string]*block.Block)
	ftBurntBlockMap[ft.TokenID] = c.w.GetLatestTokenBlock(ft.TokenID, c.TokenType(FTString))

	newBlock := block.CreateNewBlock(ftBurntBlockMap, ftBurntBlock)
	if newBlock == nil {
		return fmt.Errorf("failed to create FT burnt block")
	}

	err := newBlock.UpdateSignature(dc)
	if err != nil {
		return fmt.Errorf("failed to update FT burnt block signature")
	}

	err = c.w.AddTokenBlock(ft.TokenID, newBlock)
	if err != nil {
		return fmt.Errorf("failed to add FT burnt block")
	}

	FTTokenDetails, err := c.w.ReadFTToken(ft.TokenID)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return fmt.Errorf("FT token not found")
		}
		return fmt.Errorf("failed to read FT token details")
	}
	if FTTokenDetails != nil {
		FTTokenDetails.TokenStatus = wallet.TokenIsBurnt
		err = c.w.UpdateFTToken(FTTokenDetails)
	}
	return nil
}

// unlockParentRBTs unlocks parent RBTs after FT burning
func (c *Core) unlockParentRBTs(parentTokenID string, did string, dc did.DIDCrypto) error {
	parentTokenIDsArray := strings.Split(parentTokenID, ",")

	for _, parentRBT := range parentTokenIDsArray {
		// Get parent RBT details
		ParentRBTDetails, err := c.w.ReadToken(parentRBT)
		if err != nil {
			return fmt.Errorf("failed to get parent RBT details for %s", parentRBT)
		}

		// Create unlock block
		rbtInfo := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     parentRBT,
					TokenType: c.TokenType(RBTString),
				},
			},
			Comment: "Token Unlocked after burning FT at : " + time.Now().String(),
		}

		unlockBlock := &block.TokenChainBlock{
			TransactionType: block.TokenUnlocked,
			TokenOwner:      did,
			TransInfo:       rbtInfo,
			TokenValue:      ParentRBTDetails.TokenValue,
		}

		unlockBlockMap := make(map[string]*block.Block)
		unlockBlockMap[parentRBT] = c.w.GetLatestTokenBlock(parentRBT, c.TokenType(RBTString))

		newBlock := block.CreateNewBlock(unlockBlockMap, unlockBlock)
		if newBlock == nil {
			return fmt.Errorf("failed to create unlock block for %s", parentRBT)
		}

		err = newBlock.UpdateSignature(dc)
		if err != nil {
			return fmt.Errorf("failed to update unlock block signature for %s", parentRBT)
		}

		err = c.w.AddTokenBlock(parentRBT, newBlock)
		if err != nil {
			return fmt.Errorf("failed to add unlock block for %s", parentRBT)
		}

		// Update token status
		ParentRBTDetails.TokenStatus = wallet.TokenIsFree
		err = c.w.UpdateToken(ParentRBTDetails)
		if err != nil {
			return fmt.Errorf("failed to update token status for %s", parentRBT)
		}
	}

	return nil
}

// validateParentRBTsForBurning validates that parent RBTs and child FTs are consistent
func (c *Core) validateParentRBTsForBurning(parentTokenID string, FTsToBurn []wallet.FTToken) error {
	parentTokenIDsArray := strings.Split(parentTokenID, ",")

	// Create sorted list of FT token IDs for comparison
	var FTTokenIDsToBurn []string
	for _, ft := range FTsToBurn {
		FTTokenIDsToBurn = append(FTTokenIDsToBurn, ft.TokenID)
	}
	sort.Strings(FTTokenIDsToBurn)

	var firstChildFTs []string

	for parentTokenIDNumber, parentRBT := range parentTokenIDsArray {
		// Get parent RBT details
		_, err := c.w.ReadToken(parentRBT)
		if err != nil {
			return fmt.Errorf("failed to get parent RBT details for %s", parentRBT)
		}

		// Get latest block and child tokens
		parentRBTLatestBlock := c.w.GetLatestTokenBlock(parentRBT, c.TokenType(RBTString))
		if parentRBTLatestBlock == nil {
			return fmt.Errorf("failed to get latest block for parent RBT %s", parentRBT)
		}

		childFTs := parentRBTLatestBlock.GetChildTokens()
		if len(childFTs) != len(FTTokenIDsToBurn) {
			return fmt.Errorf("FTs count (%d) does not match with the child FTs count (%d) for RBT %s",
				len(FTTokenIDsToBurn), len(childFTs), parentRBT)
		}

		sort.Strings(childFTs)

		// Validate child FTs match the FTs to be burned
		for i := range childFTs {
			if childFTs[i] != FTTokenIDsToBurn[i] {
				return fmt.Errorf("child FTs do not match with the FTs to be burned for RBT %s", parentRBT)
			}
		}

		// Ensure consistency across all parent RBTs
		if parentTokenIDNumber == 0 {
			firstChildFTs = childFTs
		} else {
			for i := range childFTs {
				if childFTs[i] != firstChildFTs[i] {
					return fmt.Errorf("mismatch in child FTs among parent RBTs")
				}
			}
		}
	}

	return nil
}

// burnFT handles the burning of FTs with support for count-based and value-based modes
func (c *Core) burnFT(reqID string, burnReq *model.BurnFTReq) *model.BasicResponse {
	var RBTLockStatus int
	resp := &model.BasicResponse{
		Status: false,
	}

	// 1. Input validation
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(burnReq.DID)
	if !isAlphanumericDID {
		c.log.Error("Invalid sender or receiver address. Please provide valid DID")
		resp.Message = "Invalid sender or receiver address. Please provide valid DID"
		return resp
	}
	if !strings.HasPrefix(burnReq.DID, "bafybmi") || len(burnReq.DID) != 59 {
		c.log.Error("Invalid sender or receiver DID")
		resp.Message = "Invalid sender or receiver DID"
		return resp
	}
	if burnReq.FTName == "" {
		c.log.Error("FT name cannot be empty")
		resp.Message = "FT name is required"
		return resp
	}

	// 2. Validate RBTLockStatus and FTCount combination
	if burnReq.FromRBT && burnReq.FTCount > 0 {
		c.log.Error("FTCount cannot be specified when FromRBT is true, Try again with FTCount as 0")
		resp.Message = "FTCount cannot be specified when FromRBT is true, Try again with FTCount as 0"
		return resp
	}
	if !burnReq.FromRBT && burnReq.FTCount <= 0 {
		c.log.Error("FTCount is required when FromRBT is false")
		resp.Message = "FTCount is required when FromRBT is false"
		return resp
	}

	// 3. DID crypto setup
	dc, err := c.SetupDID(reqID, burnReq.DID)
	if err != nil || dc == nil {
		c.log.Error("Failed to setup DID")
		resp.Message = "DID crypto is not initialized"
		return resp
	}

	// 4. Gather all available FTs
	allFTs, err := c.w.GetFreeFTsByNameAndCreatorDID(burnReq.FTName, burnReq.DID, burnReq.DID)
	if err != nil {
		c.log.Error("Failed to get FTs to burn")
		resp.Message = "Failed to get FTs to burn"
		return resp
	}
	if len(allFTs) == 0 {
		c.log.Error("No FTs available to process")
		resp.Message = "No FTs found for burning"
		return resp
	}

	// 5. Determine FTs to burn based on FromRBT flag
	var FTsToBurn []wallet.FTToken

	if burnReq.FromRBT {
		// Burn all FTs when FromRBT is true
		FTsToBurn = allFTs
		c.log.Info("FromRBT is true, burning all available FTs")
	} else if burnReq.HighValueFT {
		targetValue := float64(burnReq.FTCount)
		var selectedFTs []wallet.FTToken
		var totalValue float64

		for _, ft := range allFTs {
			// Exact match
			if ft.TokenValue == targetValue {
				FTsToBurn = []wallet.FTToken{ft}
				c.log.Info("Found exact match FT for burning")
				break
			}

			// Accumulate FTs if their total value is still less than target
			if totalValue+ft.TokenValue < targetValue {
				selectedFTs = append(selectedFTs, ft)
				totalValue += ft.TokenValue
				continue
			}

			// This FT pushes us over the target — time to split
			remaining := targetValue - totalValue
			splitTokens, err := c.splitFTToken(dc, ft.DID, ft.TokenID, ft.TokenValue, remaining)
			if err != nil {
				c.log.Error("Failed to split FT token", "err", err)
				resp.Message = "FT splitting failed"
				return resp
			}

			// Add original accumulated tokens
			selectedFTs = append(selectedFTs, findTokenOfValue(splitTokens, remaining))
			FTsToBurn = selectedFTs
			c.log.Info(fmt.Sprintf("Split FT and burning tokens totaling %.2f", targetValue))
			break
		}

		if len(FTsToBurn) == 0 {
			c.log.Error("Insufficient FT value available for burning")
			resp.Message = fmt.Sprintf("Only total value %.2f FTs available, but %.2f requested", totalValue, targetValue)
			return resp
		}

	} else {
		// Burn by count (non-HighValue mode)
		if burnReq.FTCount > len(allFTs) {
			c.log.Error("Insufficient FTs available for burning")
			resp.Message = fmt.Sprintf("Only %d FTs available, but %d requested", len(allFTs), burnReq.FTCount)
			return resp
		}
		FTsToBurn = allFTs[:burnReq.FTCount]
		c.log.Info(fmt.Sprintf("Burning %d FTs as requested", burnReq.FTCount))
	}

	// 6. Validate all FTs before any state changes
	var parentTokenID string

	for _, ft := range FTsToBurn {
		// Get genesis block of FT
		ftGenesisBlock := c.w.GetGenesisTokenBlock(ft.TokenID, c.TokenType(FTString))
		if ftGenesisBlock == nil {
			c.log.Error(fmt.Sprintf("Failed to get genesis block for FT: %s", ft.TokenID))
			resp.Message = "Failed to get genesis block for FT"
			return resp
		}

		// Validate creator DID from genesis block
		ftCreator := ftGenesisBlock.GetOwner()
		if ftCreator != burnReq.DID {
			c.log.Error(fmt.Sprintf("DID %s is not the creator of FT %s", burnReq.DID, ft.TokenID))
			resp.Message = "Unable to burn FTs, Given DID is not the creator of the FT"
			return resp
		}

		// Validate RBT lock status when FromRBT is true
		if burnReq.FromRBT {
			RBTLockStatus, err = ftGenesisBlock.GetRBTLockStatus(ft.TokenID)
			if err != nil {
				c.log.Error("Failed to get RBT lock status")
				resp.Message = "Failed to get RBT lock status"
				return resp
			}
			if RBTLockStatus != block.RBTLocked {
				c.log.Error("RBT is not locked, cannot burn FTs")
				resp.Message = "RBT is not locked, cannot burn FTs"
				return resp
			}
		}
	}

	// 7. Additional validation for FromRBT case
	if burnReq.FromRBT || RBTLockStatus == block.RBTLocked {
		err := c.validateParentRBTsForBurning(parentTokenID, FTsToBurn)
		if err != nil {
			c.log.Error(fmt.Sprintf("Parent RBT validation failed: %v", err))
			resp.Message = err.Error()
			return resp
		}
	}

	// 8. Process parent RBT unlocking (only when FromRBT is true)
	fmt.Println("RBTLockStatus:", RBTLockStatus)
	if burnReq.FromRBT || RBTLockStatus == block.RBTLocked {
		err := c.unlockParentRBTs(parentTokenID, burnReq.DID, dc)
		if err != nil {
			c.log.Error(fmt.Sprintf("Failed to unlock parent RBTs: %v", err))
			resp.Message = "Failed to unlock parent RBTs"
			return resp
		}
	}

	// 9. Burn all FTs
	for _, ft := range FTsToBurn {
		err := c.burnSingleFT(ft, burnReq.DID, dc)
		if err != nil {
			c.log.Error(fmt.Sprintf("Failed to burn FT %s: %v", ft.TokenID, err))
			resp.Message = fmt.Sprintf("Failed to burn FT %s", ft.TokenID)
			return resp
		}
	}
	c.log.Info("FTs burned successfully")
	resp.Status = true
	resp.Message = fmt.Sprintf("Successfully burned %d %v FTs", len(FTsToBurn), burnReq.FTName)
	return resp
}

// BurnFT is the main entry point for burning FTs
func (c *Core) BurnFT(reqID string, req *model.BurnFTReq) {
	br := c.burnFT(reqID, req)
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- br
}
