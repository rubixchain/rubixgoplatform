package core

// ft.go — dead-code stub (Phase 09 replacement target)
// All FT creation and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// FTMigrationStatus describes the state of a legacy FT migration run.
type FTMigrationStatus struct {
	Status             string    `json:"status"`
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	TotalTransactions  int       `json:"total_transactions"`
	ProcessedCount     int       `json:"processed_count"`
	SuccessCount       int       `json:"success_count"`
	FailureCount       int       `json:"failure_count"`
	LastProcessedTxnID string    `json:"last_processed_txn_id"`
}

// CreateFTs stubs FT creation; replaced by InitiateTransaction in Phase 09.
func (c *Core) CreateFTs(reqID string, createFTRequest types.CreateFTReq) {
	br := model.BasicResponse{
		Status: false,
	}
	if err := c.createFTs(reqID, createFTRequest); err != nil {
		br.Message = err.Error()
	} else {
		br.Message = "FT created successfully"
		br.Status = true
	}
	channel := c.GetWebReq(reqID)
	if channel == nil {
		c.log.Error("CreateFTs: failed to get did channels")
		return
	}
	channel.OutChan <- &br
}

func (c *Core) createFTs(reqID string, req types.CreateFTReq) error {
	// validate DID inout
	if req.DID == "" {
		c.log.Error("DID is empty")
		return fmt.Errorf("core: DID is empty")
	}
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.DID)
	if !isAlphanumericDID || !strings.HasPrefix(req.DID, "bafybmi") || len(req.DID) != 59 {
		c.log.Error("core: Invalid FT creator's DID. Please provide valid DID")
		return fmt.Errorf("core: Invalid DID, Please provide valid DID")
	}

	// init DID
	didCrypto, err := c.SetupDID(reqID, req.DID)
	if err != nil || didCrypto == nil {
		c.log.Error("core: Failed to setup DID")
		return fmt.Errorf("core: DID crypto is not initialized, err: %v ", err)
	}

	// Validate input parameters
	switch {
	case req.FTCount <= 0:
		return fmt.Errorf("core: number of tokens to create must be greater than zero")
	case req.TokenCount <= 0:
		return fmt.Errorf("core: number of whole tokens must be a positive integer")
	case req.FTCount > int(req.TokenCount*1000):
		return fmt.Errorf("core: max allowed FT count is 1000 for 1 RBT")
	}

	// Fetch Whole RBTs
	//Get the RBT details from DB for the associated amount/ if token amount is of PArts create
	networkStr := "mainnet"
	if c.testnet {
		networkStr = "testnet"
	}

	// // Lock and fetch free RBT tokens for split/transfer.
	// lockedTokens, err := c.w.LockTokensForSplit(c.w.Ctx, didCryptoLib.GetDID(), deployReq.RBTAmount)
	// if err != nil {
	// 	c.log.Error("Failed to lock tokens for split", "err", err)
	// 	resp.Message = "DeploySmartContract: failed to lock tokens for split, err: " + err.Error()
	// 	return resp
	// }
	// denomMap, err := c.w.GetTokenDenomArray(didCryptoLib.GetDID())
	// if err != nil {
	// 	c.log.Error("Failed to fetch token denom array", "err", err)
	// 	resp.Message = "DeploySmartContract: failed to fetch token denom array, err: " + err.Error()
	// 	return resp
	// }
	// rbtTokensToCommitDetails, _, _, _, err := parts.CollectRBTTokens(
	// 	didCryptoLib,
	// 	c.w,
	// 	deployReq.RBTAmount,
	// 	lockedTokens,
	// 	denomMap,
	// 	networkStr,
	// 	c.log,
	// )

	// TODO: need to REPLACE it with CollectRBTTokens
	remainingTokenCount, rbtTokens, err := c.w.GetWholeRBTs(req.TokenCount, req.DID)
	if err != nil {
		return fmt.Errorf("core: RBT collection failed: %w", err)
	}
	if remainingTokenCount != 0 {
		return fmt.Errorf("core : not enough whole RBTs")
	}

	// lock rbts
	err = c.w.LockTokens(rbtTokens)
	if err != nil {
		return fmt.Errorf("core: %w", err)
	}

	// TODO: need to REMOVE after CollectRBTTokens is implemented
	var lockedRBTs []*models.TokenInfo
	for _, rbt := range rbtTokens {
		latestTxnID, err := c.w.GetLatestTransactionIdByTokenId(rbt.TokenID, false)
		if err != nil {
			return fmt.Errorf("core: failed to fetch previous txn id of token: %s, err: %w", rbt.TokenID, err)
		}
		lockedRBTs = append(lockedRBTs, &models.TokenInfo{
			TokenID: rbt.TokenID,
			TokenValue:            rbt.TokenValue,
			PreviousTransactionID: latestTxnID,
		})
	}

	// release all rbts before exiting
	defer c.w.ReleaseTokens(lockedRBTs)

	// calculate value of each FT
	ftValue, err := c.GetPreciseFractionalValue(req.TokenCount, req.FTCount)
	if err != nil {
		c.log.Error("core: Failed to calculate FT token value", err)
		return err
	}
	c.log.Debug("***** ft value ", ftValue)

	batchSizePerRBT := int(float64(1) / ftValue)
	c.log.Debug("******* batch size ", batchSizePerRBT)
	if batchSizePerRBT > int(math.Pow10(constants.MaxSupportedDecimalPlaces)) {
		return fmt.Errorf("core: per RBT division is: %d, required: <= %d", batchSizePerRBT, int(math.Pow10(constants.MaxSupportedDecimalPlaces)))
	}

	tx, err := c.w.BeginTx(c.w.Ctx)
	if err != nil {
		return fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
	}
	defer tx.Rollback(c.w.Ctx) //nolint:errcheck

	var parentTokenIDsArray []string
	for _, token := range rbtTokens {
		parentTokenIDsArray = append(parentTokenIDsArray, token.TokenID)
	}

	type ftJob struct {
		Index int
	}
	type ftResult struct {
		FTToken wallet.FTToken
		FTID    string
		Err     error
	}

	c.log.Info("core: Initializing FT creation: progress logging")
	currentTime := int(time.Now().Unix())

	// batch 100 FTs per transaction, and list of parent tokens is same for all transactions
	startIndex := req.FTNumStartIndex
	for i, parentRBT := range lockedRBTs {
		c.log.Debug("batch ", i)

		txnId, err := c.w.FTGenesisTxn(tx, didCrypto, c.ps, req.DID, networkStr, currentTime, req.FTName, req.FTNumStartIndex, batchSizePerRBT, ftValue, parentRBT)
		if err != nil {
			return fmt.Errorf("CreateFTs: failed to create genesis transaction for batch: %d, err : %w ", i, err)
		}

		logg := fmt.Sprintf("%d. txn id = %s", i, txnId)
		c.log.Debug(logg)

		// start index of next batch
		startIndex += batchSizePerRBT
	}

	// update FT count in FT table
	if _, err = tx.Exec(c.w.Ctx, `
			INSERT INTO fts (ft_name, creator_did, ft_count, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (ft_name, creator_did) DO UPDATE SET
			  ft_count = fts.ft_count + $3,
			  updated_at = NOW()
		`, req.FTName, req.DID, req.FTCount); err != nil {
		return fmt.Errorf("CreateFTs: upsert fts: %w", err)
	}

	err = tx.Commit(c.w.Ctx)
	if err != nil {
		return fmt.Errorf("core: failed persistant DB transaction, err: %w", err)
	}

	return nil
}

// InitiateFTTransfer stubs FT transfer; replaced by InitiateTransaction in Phase 09.
func (c *Core) InitiateFTTransfer(reqID string, req *model.TransferFTReq) {
	br := model.BasicResponse{
		Status:  false,
		Message: "FT transfer not yet implemented",
	}
	channel := c.GetWebReq(reqID)
	if channel == nil {
		c.log.Error("InitiateFTTransfer: failed to get did channels")
		return
	}
	channel.OutChan <- &br
}

// GetFTInfoByDID returns FT info for a DID. Stub — not yet implemented.
func (c *Core) GetFTInfoByDID(did string) ([]types.FTBalance, error) {
	ftTokenType := int16(models.GetTokenTypeID(constants.TokenType_FT))
	// get list of FT ids
	ftInfoList, err := c.w.GetTokenByDIDAndTokenType(did, ftTokenType)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get fts", "err", err)
		return []types.FTBalance{}, fmt.Errorf("failed to get fts, error: %w", err)
	}

	// map FT name and creator DID with each FT id and value
	type FTMap struct {
		FTName   string
		Creator  string
		FTValue  float64
		FTIdList []string
	}

	ftNamesMap := make(map[string]FTMap)
	for _, ft := range ftInfoList {
		// consider free FTs only
		if ft.TokenStatus != constants.TokenStatus_Free {
			continue
		}
		// split FT id
		ftParts := strings.Split(ft.TokenID, "_")
		// first and 2nd parts are ft name and creator did, respectively
		ftNameAndCreator := ftParts[0] + "_" + ftParts[1]
		ftNameAndCreatorMap := ftNamesMap[ftNameAndCreator]

		// store FT value if not stored already
		if ftNameAndCreatorMap.FTValue == float64(0) {
			ftNameAndCreatorMap.FTValue = ft.TokenValue
		}
		// store FT name and creator did if not stored already
		if ftNameAndCreatorMap.FTName == "" || ftNameAndCreatorMap.Creator == "" {
			ftNameAndCreatorMap.FTName = ftParts[0]
			ftNameAndCreatorMap.Creator = ftParts[1]
		}
		// append ft id to the lust in the map
		ftNameAndCreatorMap.FTIdList = append(ftNameAndCreatorMap.FTIdList, ft.TokenID)
	}

	// for each unique FT name and creator, calculate the number of FT Ids and to the final array to be sent back in response
	ftBalnce := []types.FTBalance{}
	for _, ftMap := range ftNamesMap {
		ftBalanceInstance := types.FTBalance{
			FTName:     ftMap.FTName,
			CreatorDID: ftMap.Creator,
			FTValue:    ftMap.FTValue,
			FTCount:    len(ftMap.FTIdList),
		}
		ftBalnce = append(ftBalnce, ftBalanceInstance)
	}
	return ftBalnce, nil
}

// FixAllFTTokensWithPeerIDAsCreator stubs legacy FT fix operation.
func (c *Core) FixAllFTTokensWithPeerIDAsCreator() ([]wallet.FTTokenFixResult, error) {
	return nil, fmt.Errorf("FixAllFTTokensWithPeerIDAsCreator: not implemented")
}

// GetFTTokenCreatorStats stubs FT creator stats retrieval.
func (c *Core) GetFTTokenCreatorStats() (map[string]interface{}, error) {
	return nil, fmt.Errorf("GetFTTokenCreatorStats: not implemented")
}

// ListFTs stubs FT listing.
func (c *Core) ListFTs() ([]*models.FT, error) {
	return c.w.ListFTs()
}

// IsAsyncFTResponse returns whether FT responses are async.
func (c *Core) IsAsyncFTResponse() bool {
	return c.cfg.AsyncFTResponse
}

// UnlockFTs stubs FT token unlock.
func (c *Core) UnlockFTs() error {
	return nil
}

// GetPreciseFractionalValue computes the fractional value of an FT.
func (c *Core) GetPreciseFractionalValue(a, b int) (float64, error) {
	if b == 0 || a == 0 {
		return 0, fmt.Errorf("GetPresiceFractionalValue: RBT value or FT count should not be zero")
	}
	result := float64(a) / float64(b)
	parts := strings.Split(strconv.FormatFloat(result, 'f', -1, 64), ".")
	decimalPlaces := 0
	if len(parts) == 2 {
		decimalPlaces = len(parts[1])
	}

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
			tempParts := strings.Split(strconv.FormatFloat(tempResult, 'f', -1, 64), ".")
			tempDecimalPlaces := 0
			if len(tempParts) == 2 {
				tempDecimalPlaces = len(tempParts[1])
			}

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
			return 0, fmt.Errorf("GetPresiceFractionalValue: FT value exceeds 3 decimal places, nearest possible value for FT count is %d", nearestB)
		} else {
			return 0, fmt.Errorf("GetPresiceFractionalValue: FT value exceeds 3 decimal places, no suitable b found within range")
		}
	}
	return result, nil
}

// GetFTMigrationStatus returns the status of the last FT migration run. Stub.
func (c *Core) GetFTMigrationStatus() (*FTMigrationStatus, error) {
	return nil, fmt.Errorf("GetFTMigrationStatus: not implemented")
}

// MigrateFTTransactionTokens stubs legacy FT migration. Stub.
func (c *Core) MigrateFTTransactionTokens() (interface{}, error) {
	return nil, fmt.Errorf("MigrateFTTransactionTokens: not implemented")
}

// GetFTTokenchain returns FT tokenchain data for a given token ID. Stub.
func (c *Core) GetFTTokenchain(tokenID string) *model.GetTokenChainResponce {
	return &model.GetTokenChainResponce{}
}

// DumpFTTokenChain returns a dump of the FT token chain. Stub.
func (c *Core) DumpFTTokenChain(req *model.TCDumpRequest) *model.TCDumpReply {
	return &model.TCDumpReply{}
}
