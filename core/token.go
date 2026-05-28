package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/token"
	tokenmap "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const defaultBatchSize = 500                             // Tweak according to RAM/network
const delayInPublshingTxnHistory = 30 * time.Millisecond // Throttle interval, tune further
const delayInPublishingTCDetails = 2 * time.Second

const subscriberBufferSize = 1000 // process up to this many idle batches
const workerCount = 8             // Tune according to hardware/network

type TokenPublish struct {
	Token string `json:"token"`
}

// TokenVerificationRequest struct
type TokenVerificationRequest struct {
	Tokens []string `json:"tokens"`
}

// TokenVerificationResponse struct
type TokenVerificationResponse struct {
	Results map[string]bool `json:"results"`
}

// Token sync info associated with Background Syncing of tokens
type TokenSyncInfo struct {
	TokenID    string  `gorm:"column:token_id;primaryKey"`
	TokenType  int     `gorm:"column:token_type"`
	AssetType  int     `gorm:"column:asset_type"`
	TokenValue float64 `gorm:"column:token_value"`
}

type PubSubEnvelope struct {
	Type string          `json:"type"` // "token" or "txn"
	Data json.RawMessage `json:"data"`
}

func (c *Core) GetAllTokens(did string, tt string) (*models.TokenResponse, error) {
	tr := &models.TokenResponse{
		BasicResponse: models.BasicResponse{
			Status:  true,
			Message: "Got all tokens",
		},
	}
	switch tt {
	case constants.TokenType_RBT:
		tkns, err := c.w.GetAllTokens(did)
		if err != nil {
			return tr, nil
		}
		tr.TokenDetails = make([]models.TokenDetail, 0)
		for _, t := range tkns {
			td := models.TokenDetail{
				Token:  t.TokenID,
				Status: int(t.TokenStatus),
			}
			tr.TokenDetails = append(tr.TokenDetails, td)
		}
	// case .NFTType:
	// 	tkns, err := c.w.GetAllNFT()
	// 	if err != nil {
	// 		return tr, nil
	// 	}
	// 	tr.TokenDetails = make([].TokenDetail, 0)
	// 	for _, t := range tkns {
	// 		td := .TokenDetail{
	// 			Token:  t.TokenID,
	// 			Status: t.TokenStatus,
	// 		}
	// 		tr.TokenDetails = append(tr.TokenDetails, td)
	// 	}
	default:
		tr.BasicResponse.Status = false
		tr.BasicResponse.Message = "Invalid token type"
	}
	return tr, nil
}

func (c *Core) GetRbtByDid(did string) (types.RBTBalance, error) {
	rbtTokenType := int64(models.GetTokenTypeID(constants.TokenType_RBT))
	wt, err := c.w.GetTokenByDIDAndTokenType(did, int16(rbtTokenType))
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens", "err", err)
		return types.RBTBalance{}, fmt.Errorf("failed to get tokens")
	}
	info := types.RBTBalance{}
	for _, t := range wt {
		switch t.TokenStatus {
		case constants.TokenStatus_Free:
			info.RBTBalance = info.RBTBalance + t.TokenValue
			info.RBTBalance = floatPrecision(info.RBTBalance, MaxDecimalPlaces)
		case constants.TokenStatus_Locked:
			info.LockedRBT = info.LockedRBT + t.TokenValue
			info.LockedRBT = floatPrecision(info.LockedRBT, MaxDecimalPlaces)
		case constants.TokenStatus_Pledged:
			info.PledgedRBT = info.PledgedRBT + t.TokenValue
			info.PledgedRBT = floatPrecision(info.PledgedRBT, MaxDecimalPlaces)
		}
	}
	return info, nil
}

func (c *Core) GenerateLocalRBT(reqID string, num int, did string, startIndex int) {
	err := c.generateLocalRBT(reqID, num, did, startIndex)
	br := models.BasicResponse{
		Status:  true,
		Message: "Local RBT tokens generated successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) GenerateMainnetRBT(reqID string, num int, did string, startIndex int) {
	err := c.generateMainnetRBT(reqID, num, did, startIndex)
	br := models.BasicResponse{
		Status:  true,
		Message: "Mainnet RBT tokens generated successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) generateMainnetRBT(reqID string, num int, did string, startIndex int) error {
	if !c.mainnet {
		return fmt.Errorf("generateMainnetRBT is only available in 'mainnet' mode")
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}

	for i := 0; i < num; i++ {
		currentTime := int(time.Now().Unix())

		tx, err := c.w.BeginTx(c.w.Ctx)
		if err != nil {
			return fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
		}
		defer tx.Rollback(c.w.Ctx) //nolint:errcheck

		globalIndex := startIndex + i
		mapLevel, numInLevel, err := tokenmap.GetMainnetTokenLevelAndNumber(globalIndex)
		if err != nil {
			return fmt.Errorf("PersistGenesisTokenRecord: GetMainnetTokenLevelAndNumber(%d): %w", globalIndex, err)
		}
		tokenID := fmt.Sprintf("%d_%d", mapLevel, numInLevel)

		if _, err = c.w.PersistGenesisTokenRecord(tx, dc, c.ps, tokenID, did, constants.NetworkMode_Mainnet, currentTime); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				c.log.Warn("Mainnet token already exists, skipping", "tokenID", tokenID)
				tx.Rollback(c.w.Ctx) //nolint:errcheck
				continue
			}
			c.log.Error("Failed to persist genesis token record", "err", err)
			return err
		}
	}

	return nil
}

// getTokenIDForLocalTestTokens retrieves the token ID for local test tokens based on the
// provided token level and token number.
func (c *Core) getTokenIDForLocalTestTokens(tokenLevel int, tokenNumber int) (string, error) {
	idValue := strconv.Itoa(tokenLevel) + "_" + strconv.Itoa(tokenNumber)
	return idValue, nil
}

func (c *Core) generateLocalRBT(reqID string, num int, did string, startIndex int) error {
	if !c.localnet {
		return fmt.Errorf("generate test token is available in 'localnet' mode. Run rubix in localnet mode by providing -localnet flag")
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}

	// TokenID is assigned atomically inside PersistGenesisTokenRecord using the
	// global DB counter (GetNextTokenNumber).
	if startIndex == 0 {
		for i := 0; i < num; i++ {
			currentTime := int(time.Now().Unix())

			tx, err := c.w.BeginTx(c.w.Ctx)
			if err != nil {
				return fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
			}
			defer tx.Rollback(c.w.Ctx) //nolint:errcheck

			// Get the tokenID based on a canonical index
			globalIndex, err := c.w.GetNextTokenNumber(c.w.Ctx, tx)
			if err != nil {
				return fmt.Errorf("PersistGenesisTokenRecord: GetNextTokenNumber: %w", err)
			}
			tokenLevel, numInLevel, err := tokenmap.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
			if err != nil {
				return fmt.Errorf("PersistGenesisTokenRecord: GetTokenLevelAndNumberForGlobalIndex(%d): %w", globalIndex, err)
			}
			tokenID := fmt.Sprintf("%d_%d", tokenLevel, numInLevel)

			if _, err = c.w.PersistGenesisTokenRecord(tx, dc, c.ps, tokenID, did, constants.NetworkMode_Localnet, currentTime); err != nil {
				c.log.Error("Failed to persist genesis token record", "err", err)
				return err
			}
		}
	} else {
		startTokenNumber := startIndex
		finalTokenNumber := startTokenNumber + num

		for globalIndex := startTokenNumber; globalIndex < finalTokenNumber; globalIndex++ {
			currentTime := int(time.Now().Unix())

			tx, err := c.w.BeginTx(c.w.Ctx)
			if err != nil {
				return fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
			}
			defer tx.Rollback(c.w.Ctx) //nolint:errcheck

			tokenLevel, numInLevel, err := tokenmap.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
			if err != nil {
				return fmt.Errorf("PersistGenesisTokenRecord: GetTokenLevelAndNumberForGlobalIndex(%d): %w", globalIndex, err)
			}
			tokenID := fmt.Sprintf("%d_%d", tokenLevel, numInLevel)

			if _, err = c.w.PersistGenesisTokenRecord(tx, dc, c.ps, tokenID, did, constants.NetworkMode_Localnet, currentTime); err != nil {
				c.log.Error("Failed to persist genesis token record", "err", err)
				return err
			}
		}
	}

	return nil
}

func (c *Core) getFromIPFS(path string) ([]byte, error) {
	rpt, err := c.ipfsOps.Cat(path)
	if err != nil {
		c.log.Error("failed to get from ipfs", "err", err, "path", path)
		return nil, err
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(rpt)
	b := buf.Bytes()
	return b, nil
}

func (c *Core) GetpinnedTokens(did string) ([]models.Token, error) {
	requiredTokens, err := c.w.GetAllPinnedTokens(did)
	if err != nil {
		c.log.Error("Error retrieving pinned tokens from database :", err)
		return nil, err
	}
	return requiredTokens, nil
}

func (c *Core) GenerateFaucetTestTokens(reqID string, tokenCount int, did string) {

	br := models.BasicResponse{
		Status:  true,
		Message: "",
	}

	tokenDetails, err := c.generateTestRBT(reqID, tokenCount, did)
	if err != nil {
		c.log.Error("Failed to get token details from generateTestTokensFaucet", "err", err)
		br.Status = false
		br.Message = br.Message + ",  " + err.Error()
		return
	}

	//If an error occurs at any given time, and the tokens have been created for that, reduce the latest token number by 1
	// if err != nil {
	// 	br.Status = false
	// 	br.Message = err.Error()
	// 	tokenDetails.CurrentTokenNumber = tokenDetails.CurrentTokenNumber - 1
	// 	if tokenDetails.CurrentTokenNumber == 0 && tokenDetails.TokenLevel != 1 {
	// 		tokenDetails.TokenLevel = tokenDetails.TokenLevel - 1
	// 	}
	// }

	// Send a POST request to update the token details to the faucet server
	jsonData, err := json.Marshal(tokenDetails)
	if err != nil {
		c.log.Error("Error marshaling JSON:", "err", err)
		br.Status = false
		br.Message = br.Message + ",  " + err.Error()
		return
	}
	u, _ := url.Parse(c.faucetURL)
	u.Path = path.Join(u.Path, "/api/update-token-value")
	updatedTokenValueURL := u.String()

	resp, err := http.Post(updatedTokenValueURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		c.log.Error("Failed to update latest token number in Faucet", "err", err)
		br.Status = false
		br.Message = br.Message + ",  " + err.Error()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		br.Message = br.Message + ",  " + "Successfully updated token details."
	} else {
		br.Status = false
		br.Message = br.Message + ",  " + "Failed to update token details. Status code:" + strconv.Itoa(resp.StatusCode)
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) getTestTokensID(tokenLevel int, tokenNumber int) (string, error) {
	idStrVal := fmt.Sprintf("%d_%d", tokenLevel, tokenNumber)
	return idStrVal, nil
}

func (c *Core) generateTestRBT(reqID string, numTokens int, did string) (*token.FaucetToken, error) {
	if !c.testnet {
		return nil, fmt.Errorf("generate test token is available in test net")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return nil, fmt.Errorf("DID does not exist")
	}

	u, _ := url.Parse(c.faucetURL)

	u.Path = path.Join(u.Path, "/api/current-token-value")

	getTokenValueURL := u.String()
	// Get the current value from Faucet
	resp, err := http.Get(getTokenValueURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokendetail token.FaucetToken

	body, err := io.ReadAll(resp.Body)
	//Populating the tokendetail with current token number and current token level received from Faucet.
	json.Unmarshal(body, &tokendetail)
	if err != nil {
		return nil, err
	}
	//Updating the Faucet token details with each new token
	for i := 0; i < numTokens; i++ {
		currentTime := int(time.Now().Unix())

		tx, err := c.w.BeginTx(c.w.Ctx)
		if err != nil {
			return nil, fmt.Errorf("PersistGenesisTokenRecord: begin tx: %w", err)
		}
		defer tx.Rollback(c.w.Ctx) //nolint:errcheck

		tokendetail.CurrentTokenNumber += 1

		//If the latest token number to be generated is more than the max token value of previous token, increase the token level
		levelOffset := tokendetail.TokenLevel - constants.TestnetRBT_Level_Offset
		maxTokens := token.TokenMap[levelOffset]
		if tokendetail.CurrentTokenNumber == maxTokens+1 {
			tokendetail.TokenLevel += 1
			tokendetail.CurrentTokenNumber = 1
		}

		id, err := c.getTestTokensID(tokendetail.TokenLevel, tokendetail.CurrentTokenNumber)
		if err != nil {
			c.log.Error("Failed to get token ID from IPFS", "err", err)
			return &tokendetail, fmt.Errorf("failed to get token ID from IPFS")
		}

		if _, err = c.w.PersistGenesisTokenRecord(tx, dc, c.ps, id, did, constants.NetworkMode_Testnet, currentTime); err != nil {
			c.log.Error("Failed to persist genesis token record", "err", err)
			return &tokendetail, err
		}

		tokendetail.TotalCount += 1
	}

	return &tokendetail, nil
}

// VerifyTokens function sends the API request and handles the response
func VerifyTokens(serverURL string, tokens []string) (TokenVerificationResponse, error) {
	url := fmt.Sprintf("%s/verify", serverURL)

	requestBody := TokenVerificationRequest{Tokens: tokens}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return TokenVerificationResponse{}, fmt.Errorf("error marshalling request: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return TokenVerificationResponse{}, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TokenVerificationResponse{}, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	var responseBody TokenVerificationResponse
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		return TokenVerificationResponse{}, fmt.Errorf("error decoding response: %v", err)
	}

	return responseBody, nil

}

// GetMissingBlockSequence validates the full PostgreSQL tokenchain for the given token.
// Returns nil if the chain is complete and consistent; returns an error describing the first
// detected gap or linkage break. The block-based min/max ID concept has been removed —
// chain integrity is now defined by position sequentiality and previous_transaction_id linkage.
func (c *Core) GetMissingBlockSequence(tokenSyncInfo TokenSyncInfo) error {
	chain, err := c.w.GetTokenChainByTokenID(tokenSyncInfo.TokenID, false)
	if err != nil {
		c.log.Error("Failed to fetch token chain", "token", tokenSyncInfo.TokenID, "error", err)
		return fmt.Errorf("failed to fetch token chain for %s: %w", tokenSyncInfo.TokenID, err)
	}

	if len(chain) == 0 {
		c.log.Error("Token chain is empty", "token", tokenSyncInfo.TokenID)
		return fmt.Errorf("missing token chain for token: %s", tokenSyncInfo.TokenID)
	}

	for i := 1; i < len(chain); i++ {
		// Position must be strictly sequential
		if chain[i].Position != chain[i-1].Position+1 {
			c.log.Error("Token chain position gap", "token", tokenSyncInfo.TokenID,
				"position", chain[i].Position, "prev_position", chain[i-1].Position)
			return fmt.Errorf("tokenchain gap at position %d (expected %d) for token %s",
				chain[i].Position, chain[i-1].Position+1, tokenSyncInfo.TokenID)
		}
		// previous_transaction_id must match the prior entry's transaction_id
		prev := chain[i].PreviousTransactionID
		if prev == nil || *prev != chain[i-1].TransactionID {
			c.log.Error("Token chain linkage broken", "token", tokenSyncInfo.TokenID,
				"position", chain[i].Position, "expected_prev", chain[i-1].TransactionID, "got_prev", prev)
			return fmt.Errorf("broken tokenchain linkage at position %d for token %s",
				chain[i].Position, tokenSyncInfo.TokenID)
		}
	}

	return nil
}
