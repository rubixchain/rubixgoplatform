package core

// ft.go — dead-code stub (Phase 09 replacement target)
// All FT creation and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
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
		c.log.Error(err.Error())
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

func (c *Core) createFTs(reqID string, req types.CreateFTReq) (err error) {
	// validate DID inout
	if req.DID == "" {
		c.log.Error("DID is empty")
		return fmt.Errorf("createFTs: DID is empty")
	}
	isAlphanumericDID := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(req.DID)
	if !isAlphanumericDID || !strings.HasPrefix(req.DID, "bafybmi") || len(req.DID) != 59 {
		c.log.Error("createFTs: Invalid FT creator's DID. Please provide valid DID")
		return fmt.Errorf("createFTs: Invalid DID, Please provide valid DID")
	}

	// init DID
	didCrypto, err := c.SetupDID(reqID, req.DID)
	if err != nil || didCrypto == nil {
		c.log.Error("createFTs: Failed to setup DID")
		return fmt.Errorf("createFTs: DID crypto is not initialized, err: %v ", err)
	}

	// Validate input parameters
	switch {
	case req.FTCount <= 0:
		return fmt.Errorf("createFTs: number of tokens to create must be greater than zero")
	case req.TokenCount <= 0:
		return fmt.Errorf("createFTs: number of whole tokens must be a positive integer")
	case req.FTCount > int(req.TokenCount*1000):
		return fmt.Errorf("createFTs: max allowed FT count is 1000 for 1 RBT")
	}

	// Fetch Whole RBTs
	//Get the RBT details from DB for the associated amount/ if token amount is of PArts create
	networkMode := "mainnet"
	if c.testnet {
		networkMode = "testnet"
	}

	// Lock and fetch free RBT tokens for split/transfer.
	lockedTokens, err := c.w.LockTokensForSplit(c.w.Ctx, req.DID, float64(req.TokenCount), reqID)
	if err != nil {
		err = fmt.Errorf("createFTs:Failed to lock tokens for split, err: %v", err)
		c.log.Error(err.Error())
		return
	}
	denomMap, err := c.w.GetTokenDenomArray(req.DID)
	if err != nil {
		err = fmt.Errorf("createFTs: Failed to fetch token denom array, err: %v", err)
		c.log.Error(err.Error())
		return
	}
	rbtTokens, childTokensKept, burntParentToken, mintTokensBeingBurnt, err := parts.CollectRBTTokens(
		didCrypto,
		c.w,
		float64(req.TokenCount),
		lockedTokens,
		denomMap,
		networkMode,
		c.log,
	)
	if err != nil {
		err = fmt.Errorf("createFTs: failed to collect RBT tokens: %w", err)
		c.log.Error(err.Error())
		return
	}

	// release all rbts before exiting with error
	defer func() {
		if err != nil {
			c.w.ReleaseTokens(rbtTokens, reqID)
		}
	}()

	// if any RBT was split, add the txn details to DB
	if len(childTokensKept) > 0 && len(burntParentToken) > 0 {
		genTX := childTokensKept[0].TxRecord
		if genTX == nil {
			err = fmt.Errorf("createFTs: generated transaction record is nil")
			return
		}

		if errPersist := c.w.PersistGenesisTransaction(&wallet.PersistGenesisTransactionReq{
			DID:                  req.DID,
			GenesisTokens:        childTokensKept,
			BurnTokens:           burntParentToken,
			GenesisTransaction:   genTX,
			MintTokensBeingBurnt: mintTokensBeingBurnt,
		}); errPersist != nil {
			err = fmt.Errorf("createFTs: failed to persist genesis transaction, err: %v", errPersist)
			c.log.Error(err.Error())
			return
		}

		var txInfo models.TransactionInfo
		if err = json.Unmarshal(genTX.Info, &txInfo); err != nil {
			err = fmt.Errorf("createFTs: failed to unmarshal transaction info, err: %v", err)
			c.log.Error(err.Error())
			return
		}

		var txSingature models.Signature
		if err = json.Unmarshal(genTX.Signature, &txSingature); err != nil {
			err = fmt.Errorf("createFTs: failed to unmarshal signature, err: %v", err)
			c.log.Error(err.Error())
			return
		}

		if networkMode != constants.NetworkMode_Localnet {
			if _, pubErr := util.PublishTransaction(c.ps, &txInfo, &txSingature, true, ""); pubErr != nil {
				c.log.Error("createFTs: failed to publish transaction, err: %v", pubErr)
			}
		}
	}

	if len(rbtTokens) == 0 {
		err = fmt.Errorf("createFTs: empty list of RBT tokens: %w", err)
		c.log.Error(err.Error())
		return
	}

	// calculate value of each FT
	ftValue, err := c.GetPreciseFractionalValue(req.TokenCount, req.FTCount)
	if err != nil {
		err = fmt.Errorf("createFTs: Failed to calculate FT token value : %v", err)
		c.log.Error(err.Error())
		return
	}

	batchSizePerRBT := int(float64(1) / ftValue)
	if batchSizePerRBT > int(math.Pow10(constants.MaxSupportedDecimalPlaces)) {
		err = fmt.Errorf("core: per RBT division is: %d, required: <= %d", batchSizePerRBT, int(math.Pow10(constants.MaxSupportedDecimalPlaces)))
		c.log.Error(err.Error())
		return
	}

	tx, err := c.w.BeginTx(c.w.Ctx)
	if err != nil {
		err = fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
		c.log.Error(err.Error())
		return
	}
	defer tx.Rollback(c.w.Ctx) //nolint:errcheck

	c.log.Info("core: Initializing FT creation: progress logging")
	currentTime := int(time.Now().Unix())

	// update FT info in FT table
	var ftsID int32
	err = tx.QueryRow(c.w.Ctx,
		`INSERT INTO fts (ft_name, creator_did, ft_count, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (ft_name, creator_did) DO UPDATE SET
			ft_count   = fts.ft_count + $3,
			updated_at = NOW()
		RETURNING id`,
		req.FTName, req.DID, req.FTCount,
	).Scan(&ftsID)
	if err != nil {
		err = fmt.Errorf("CreateFTs: upsert fts: %w", err)
		c.log.Error(err.Error())
		return
	}

	// batch RBTs such that each batch has sum of TokenValue == 1.0
	rbtBatches, err := c.BatchRBTs(rbtTokens)
	if err != nil {
		err = fmt.Errorf("CreateFTs: failed to create rbt batches, err : %v", err)
		c.log.Error(err.Error())
		return
	}
	if rbtBatches == nil {
		err = fmt.Errorf("CreateFTs: empty rbt batches ")
		c.log.Error(err.Error())
		return
	}
	// batch FTs by one transaction per RBT
	startIndex := req.FTNumStartIndex
	for i, parentRBTs := range rbtBatches {

		txnId, gterr := c.w.FTGenesisTxn(tx, didCrypto, c.ps, req.DID, networkMode, currentTime, req.FTName, startIndex, batchSizePerRBT, ftValue, ftsID, parentRBTs)
		if gterr != nil {
			err = fmt.Errorf("CreateFTs: failed to create genesis transaction for batch: %d, err : %w ", i, gterr)
			c.log.Error(err.Error())
			return
		}

		logg := fmt.Sprintf("%d. txn id = %s", i, txnId)
		c.log.Debug(logg)

		// start index of next batch
		startIndex += batchSizePerRBT
	}

	err = tx.Commit(c.w.Ctx)
	if err != nil {
		err = fmt.Errorf("core: failed persistant DB transaction, err: %w", err)
		c.log.Error(err.Error())
		return
	}

	return nil
}

// Group rbtTokens into batches where sum of TokenValue == 1.0
func (c *Core) BatchRBTs(rbtTokens []*models.TokenInfo) (rbtBatches [][]*models.TokenInfo, err error) {
	var currentBatch []*models.TokenInfo
	currentSum := 0.0

	for _, token := range rbtTokens {
		currentBatch = append(currentBatch, token)
		currentSum = rubixmath.AddFloat(currentSum, token.TokenValue)
		if currentSum == 1.0 { // sum reached 1.0
			rbtBatches = append(rbtBatches, currentBatch)
			currentBatch = nil
			currentSum = 0.0
		}
	}

	// handle leftover tokens that don't sum to 1.0
	if len(currentBatch) > 0 {
		err = fmt.Errorf("CreateFTs: tokens do not sum to whole RBT, leftover sum: %v", currentSum)
		c.log.Error(err.Error())
		return
	}

	return
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
