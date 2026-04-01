package core

// ft.go — dead-code stub (Phase 09 replacement target)
// All FT creation and transfer logic will be replaced by InitiateTransaction.
// These stubs satisfy server/ call sites until the replacement is wired.

import (
	"fmt"
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
func (c *Core) CreateFTs(reqID string, did string, ftcount int, ftname string, wholeToken int, ftNumStartIndex int) {
	br := model.BasicResponse{
		Status:  false,
		Message: "FT creation not yet implemented",
	}
	channel := c.GetWebReq(reqID)
	if channel == nil {
		c.log.Error("CreateFTs: failed to get did channels")
		return
	}
	channel.OutChan <- &br
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
	return nil, fmt.Errorf("ListFTs: not implemented")
}

// IsAsyncFTResponse returns whether FT responses are async.
func (c *Core) IsAsyncFTResponse() bool {
	return c.cfg.AsyncFTResponse
}

// UnlockFTs stubs FT token unlock.
func (c *Core) UnlockFTs() error {
	return nil
}

// GetPresiceFractionalValue computes the fractional value of an FT.
func (c *Core) GetPresiceFractionalValue(a, b int) (float64, error) {
	if b <= 0 {
		return 0, fmt.Errorf("GetPresiceFractionalValue: divisor must be positive")
	}
	return float64(a) / float64(b), nil
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
