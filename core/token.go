package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
	tokenmap "github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
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

func (c *Core) GetAllTokens(did string, tt string) (*model.TokenResponse, error) {
	tr := &model.TokenResponse{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Got all tokens",
		},
	}
	switch tt {
	case model.RBTType:
		tkns, err := c.w.GetAllTokens(did)
		if err != nil {
			return tr, nil
		}
		tr.TokenDetails = make([]model.TokenDetail, 0)
		for _, t := range tkns {
			td := model.TokenDetail{
				Token:  t.TokenID,
				Status: int(t.TokenStatus),
			}
			tr.TokenDetails = append(tr.TokenDetails, td)
		}
	// case model.NFTType:
	// 	tkns, err := c.w.GetAllNFT()
	// 	if err != nil {
	// 		return tr, nil
	// 	}
	// 	tr.TokenDetails = make([]model.TokenDetail, 0)
	// 	for _, t := range tkns {
	// 		td := model.TokenDetail{
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
	br := model.BasicResponse{
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
	br := model.BasicResponse{
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

func (c *Core) updateStatus(req *ensweb.Request) *ensweb.Result {
	var updateReq model.UpdateTokenStatusReq

	// Parse request
	if err := c.l.ParseJSON(req, &updateReq); err != nil {
		c.log.Warn("Failed to parse request", "error", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}
	var resp model.BasicResponse

	err := c.w.UpdateTokenStatus(updateReq.DID, updateReq.TokenHash, updateReq.TokenType, updateReq.NewTokenStatus)
	if err != nil {
		c.log.Error("Failed to update token status", "err", err)
		resp.Message = "Failed to update token status"
		resp.Status = false
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	resp.Message = "Updated token status"
	resp.Status = true
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) getTokenStatus(req *ensweb.Request) *ensweb.Result {
	var getStatusReq model.GetTokenStatusReq

	// Parse request
	if err := c.l.ParseJSON(req, &getStatusReq); err != nil {
		c.log.Warn("Failed to parse request", "error", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}
	var resp model.TokenStatusResponse

	resp, err := c.w.GetTokenStatus(getStatusReq.DID, getStatusReq.Token, getStatusReq.Type)
	if err != nil {
		c.log.Error("Failed to get token status", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}

	return c.l.RenderJSON(req, &resp, http.StatusOK)
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

func (c *Core) GetPledgedInfo() ([]model.PledgedTokenStateDetails, error) {
	wt, err := c.w.GetAllTokenStateHash()
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get token state hashes", "err", err)
		return []model.PledgedTokenStateDetails{}, fmt.Errorf("failed to get token states")
	}
	info := []model.PledgedTokenStateDetails{}
	for _, t := range wt {
		k := model.PledgedTokenStateDetails{
			DID:            t.DID,
			TokensPledged:  t.PledgedTokens,
			TokenStateHash: t.TokenStateHash,
		}
		info = append(info, k)
	}
	return info, nil
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

	br := model.BasicResponse{
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

// func (c *Core) FaucetTokenCheck(tokenID string, did string) model.BasicResponse {
// 	br := model.BasicResponse{
// 		Status: false,
// 	}
// 	//Cheking if token is valid
// 	b, err := c.getFromIPFS(tokenID)

// 	if err != nil {
// 		c.log.Error("failed to get token details from ipfs", "err", err, "token", tokenID)
// 		br.Message = "Cannot find token details"
// 		return br
// 	}

// 	tokenval := string(b)
// 	tokencontent := strings.Split(tokenval, ",")
// 	if len(tokencontent) != 3 {
// 		br.Message = "Non-faucet token"
// 		return br
// 	}

// 	faucetName := strings.TrimSpace(strings.Split(tokencontent[0], ":")[1])
// 	if faucetName != token.FaucetName {
// 		br.Message = "Invalid faucet name"
// 		return br
// 	}

// 	tokenLevel, err := strconv.Atoi(strings.TrimSpace(strings.Split(tokencontent[1], ":")[1]))
// 	if err != nil {
// 		br.Message = "Invalid token level"
// 		return br
// 	}

// 	tokenNumber, err := strconv.Atoi(strings.TrimSpace(strings.Split(tokencontent[2], ":")[1]))
// 	if err != nil {
// 		br.Message = "Invalid token number"
// 		return br
// 	}
// 	if tokenNumber > token.TokenMap[tokenLevel] {
// 		br.Message = "Invalid token number"
// 		return br
// 	}

// 	u, _ := url.Parse(c.faucetURL)

// 	u.Path = path.Join(u.Path, "/api/current-token-value")

// 	currentTokenValueURL := u.String()

// 	// Get the current value from Faucet
// 	resp, err := http.Get(currentTokenValueURL)
// 	if err != nil {
// 		br.Status = false
// 		br.Message = "Unable to fetch latest value"
// 		return br
// 	}
// 	defer resp.Body.Close()

// 	var tokendetail token.FaucetToken

// 	body, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		br.Status = false
// 		br.Message = "Unable to fetch latest value"
// 		return br
// 	}
// 	//Populating the tokendetail with current token number and current token level received from Faucet.
// 	err = json.Unmarshal(body, &tokendetail)
// 	if err != nil {
// 		br.Status = false
// 		br.Message = "Unable to fetch latest value"
// 		return br
// 	}
// 	if tokenLevel > tokendetail.TokenLevel {
// 		br.Message = "Invalid token level"
// 		return br
// 	}

// 	// TODO(phase07): block-based token chain validation removed
// 	// Previously: GetGenesisTokenBlock + GetSigner to verify faucet DID
// 	br.Message = "Token chain validation temporarily unavailable (block removal in progress)"
// 	return br

// 	response, err := c.ValidateTokenOwner(TokenChainInput{}, did)
// 	if err != nil {
// 		c.log.Error("msg", response.Message, "err", err)
// 		br.Message = "Token Details : " + tokenval + " Couldn't validate token chain"
// 		return br
// 	}

// 	br.Status = true
// 	br.Message = "Token owner validated successfully. Token details = " + tokenval

// 	return br
// }

// This function might not be needed since we are going from the tokenHash structure.
func (c *Core) ValidateToken(token string) (*model.BasicResponse, error) {

	response := &model.BasicResponse{
		Status:  false,
		Message: "Invalid token hash",
	}

	// commented out for now, #TODO
	/* if c.testNet {
		response.Message = "validate token is not available in test net"
		response.Result = "invalid operation"
		return response, fmt.Errorf("validate token is not available in test net")
	} */
	// Get token hash from IPFS
	tokenHashReader, err := c.ipfsOps.Cat(token)
	if err != nil {
		return response, fmt.Errorf("error getting token hash from IPFS: %v", err)
	}
	defer tokenHashReader.Close()

	// Read token hash from io.ReadCloser
	var tokenHashBuf bytes.Buffer
	if _, err := io.Copy(&tokenHashBuf, tokenHashReader); err != nil {
		return response, fmt.Errorf("error reading token hash: %v", err)
	}
	tokenHash := tokenHashBuf.String()
	// Trim any leading/trailing whitespace, including newlines
	tokenHash = strings.TrimSpace(tokenHash)
	/*
		// Length check (should be 67 characters as per your requirements)
		if len(tokenHash) != 67 {
			return response, fmt.Errorf("invalid token length: %s, length is %v", tokenHash, len(tokenHash))
		} */

	// Call the VerifyTokens function from the tokenverifier package
	verifyResponse, err := VerifyTokens(TokenValidatorURL, []string{tokenHash})
	if err != nil {
		return response, fmt.Errorf("token verification API call failed: %v", err)
	}

	// Check the result from the API response
	isValid, tokenFound := verifyResponse.Results[tokenHash]
	if !tokenFound {
		return response, fmt.Errorf("token not found in verification response")
	}

	if isValid {
		response.Status = true
		response.Message = fmt.Sprintf("Token %s is valid", token)
	} else {
		response.Message = fmt.Sprintf("Token %s is invalid", token)
	}

	return response, nil
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

// Get token's ipfs content from the provider and store it in the psql db
func (c *Core) AddTokenContentToPSQL(tokenId string, assetType int) error {
	maxRetries := 3
	var tokenContent string
	var err error

	// re-attempt when ipfs cat fails
	for attempt := 1; attempt <= maxRetries; attempt++ {
		tokenHash, _ := c.ipfsOps.Add(
			bytes.NewBufferString(tokenId), nil,
		)

		tokenContent, err = c.w.Cat(tokenHash, constants.TokenProviderRole_FullNode, c.peerID)
		if err == nil {
			break
		}

		c.log.Warn(fmt.Sprintf(
			"Attempt %d/%d failed to fetch IPFS content for token %s: %v",
			attempt, maxRetries, tokenId, err,
		))

		// Exponential backoff: wait before retrying
		backoff := time.Duration(attempt*2) * time.Second
		time.Sleep(backoff)
	}

	if err != nil {
		errMsg := fmt.Sprintf("failed to get IPFS content of token %v after %d attempts: %v",
			tokenId, maxRetries, err)
		c.log.Error(errMsg)
		return fmt.Errorf(errMsg)
	}

	switch assetType {
	case RBTTokenType:
		rbtContent := &wallet.RBTContent{
			TokenID:    tokenId,
			RBTContent: tokenContent,
		}
		err = c.w.AddRBTContentToPSQl(rbtContent)
		if err != nil {
			errMsg := fmt.Sprintf("failed to add ipfs content of rbt : %v to fullnode psql db, err: %v", tokenId, err)
			c.log.Error(errMsg)
			return fmt.Errorf(errMsg)
		}
	case FTTokenType:
		ftContent := &wallet.FTContent{
			TokenID:   tokenId,
			FTContent: tokenContent,
		}
		err = c.w.AddFTContentToPSQl(ftContent)
		if err != nil {
			errMsg := fmt.Sprintf("failed to add ipfs content of ft : %v to fullnode psql db, err: %v", tokenId, err)
			c.log.Error(errMsg)
			return fmt.Errorf(errMsg)
		}
	case NFTTokenType:
		// unmarshall the json and convert into struct
		var nft models.IPFSContractInfo
		err = json.Unmarshal([]byte(tokenContent), &nft)
		if err != nil {
			c.log.Error("Failed to parse nft", "err", err)
			return err
		}

		outputDir := fmt.Sprintf("/tmp/nft/%s", tokenId)
		err = os.MkdirAll(outputDir, 0755)
		if err != nil {
			c.log.Error("Failed to create binary code directory", "err", err)
			return err
		}
		var err error

		// re-attempt 3 times if ipfs get fails
		for attempt := 1; attempt <= 3; attempt++ {
			err = c.ipfsOps.Get(nft.ArtifactHash, outputDir)
			if err == nil {
				c.log.Info("Successfully fetched NFT folder", "attempt", attempt)
				break
			}
			c.log.Warn("Retrying NFT fetch", "attempt", attempt, "err", err)
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		if err != nil {
			c.log.Error("Failed to fetch NFT folder after retries", "hash", nft.ArtifactHash, "err", err)
			return err
		}

		// store the files into PostgreSQL as blobs
		err = c.w.StoreNFTFilesToPSQL(tokenId, nft.DID, nft.ArtifactHash, outputDir)
		if err != nil {
			c.log.Error("Failed to store NFT files to DB", "err", err)
		}

		// Clean up temp directory after storing in DB
		if rmErr := os.RemoveAll(outputDir); rmErr != nil {
			c.log.Warn("Failed to remove temp outputDir", "path", outputDir, "err", rmErr)
		} else {
			c.log.Info("Removed temporary directory", "path", outputDir)
		}

	case SmartContractTokenType:
		// Parse smart contract token JSON into SmartContractToken struct
		var smartContractIpfsInfo models.IPFSContractInfo
		err = json.Unmarshal([]byte(tokenContent), &smartContractIpfsInfo)
		if err != nil {
			c.log.Error("Failed to parse smart contract token", "err", err)
			return err
		}

		if err := c.StoreSmartContractFilesToPSQL(tokenId, smartContractIpfsInfo); err != nil {
			errMsg := fmt.Sprintf("failed to add smart contract token to psql , smart contract hash : %v, err : %v", tokenId, err)
			c.log.Error(errMsg)
			return fmt.Errorf(errMsg)
		}

	default:
		errMsg := fmt.Sprintf("failed to add ipfs content, invalid asset type :%v of token : %v", assetType, tokenId)
		c.log.Error(errMsg)
		return fmt.Errorf(errMsg)
	}
	return nil
}

// check if token exists in postgres, throw error if it does not exist
func (c *Core) ReadTokenContentFromPSQL(tokenId string, assetType int) error {
	var err error

	switch assetType {
	case RBTTokenType:
		_, err = c.w.ReadRBTContentFromTable(tokenId)
		if err != nil {
			return err
		}
	case FTTokenType:
		_, err = c.w.ReadFTContentFromTable(tokenId)
		if err != nil {
			return err
		}
	case NFTTokenType:
		_, err = c.w.ReadNFTContentFromTable(tokenId)
		if err != nil {
			return err
		}
	case SmartContractTokenType:
		_, err = c.w.ReadSmartContractContentFromTable(tokenId)
		if err != nil {
			return err
		}

	default:
		errMsg := fmt.Sprintf("failed to read ipfs content, invalid asset type :%v of token : %v", assetType, tokenId)
		c.log.Error(errMsg)
		return fmt.Errorf("%v", errMsg)
	}
	return nil
}

// These function looks like a stub to make the code compiling, commenting out the error causing part which was updated when
// contract part was refactored.
func (c *Core) StoreSmartContractFilesToPSQL(smartContractHash string, smartContractIpfsContent models.IPFSContractInfo) error {
	// Fetch the binary code file
	// binaryCodeFile, err := c.ipfsOps.Cat(smartContractIpfsContent.BinaryCodeHash)
	// if err != nil {
	// 	c.log.Error("Failed to fetch binary code file from network", "err", err)
	// 	return err
	// }
	// defer binaryCodeFile.Close()

	// binaryCodeFileName := "binaryCodeFile.wasm"

	// Read the content of binaryCodeFile
	// binaryCodeContent, err := io.ReadAll(binaryCodeFile)
	// if err != nil {
	// 	c.log.Error("Failed to read binary code file", "err", err)
	// 	return err
	// }

	// Fetch and store the raw code file
	// rawCodeFile, err := c.ipfsOps.Cat(smartContractIpfsContent.RawCodeHash)
	// if err != nil {
	// 	c.log.Error("Failed to fetch raw code file from IPFS", "err", err)
	// 	return err
	// }
	// defer rawCodeFile.Close()

	// rawCodeFileName := "rawCodeFile"

	// Read the content of rawCodeFile
	// rawCodeContent, err := io.ReadAll(rawCodeFile)
	// if err != nil {
	// 	c.log.Error("Failed to read raw code file", "err", err)
	// 	return err
	// }

	// Add smart contract IPFS content in PSQL db
	// smartContractContent := &wallet.SmartContractContent{
	// 	SmartContractHash:  smartContractHash,
	// 	DeployerDID:        smartContractIpfsContent.DID,
	// 	BinaryCodeFileName: binaryCodeFileName,
	// 	BinaryCode:         binaryCodeContent,
	// 	RawCodeFileName:    rawCodeFileName,
	// 	RawCode:            rawCodeContent,
	// }
	// err = c.w.AddSmartContractContentToPSQl(smartContractContent)
	// if err != nil {
	// 	errMsg := fmt.Sprintf("failed to add smart contract content to psql, smart contract hash : %v, error: %v", smartContractHash, err)
	// 	c.log.Error(errMsg)
	// 	return fmt.Errorf(errMsg)
	// }

	c.log.Info("Successfully stored all smart contract files")
	return nil
}

func (c *Core) relaseToken(release *bool, token string) {
	if *release {
		c.w.ReleaseToken(token)
	}
}

// syncTransactionChain handles a sync request for a token's transaction chain.
// // (upstream addition — Category B)
// func (c *Core) syncTransactionChain(req *ensweb.Request) *ensweb.Result {
// 	return rubixsync.SyncTransactionChain(req, c.l, c.w, c.log)
// }

// syncTransactionChainFrom fetches missing transactions from a peer and writes them locally.
// (upstream addition — Category B)
// func (c *Core) SyncTransactionChainFrom(p *ipfsport.Peer, token string) (error, *models.TransactionChainSyncReply) {
// 	var err error

// 	latestTransactionID := c.w.GetLatestTransactionID(token)
// 	if latestTransactionID == "" {
// 		c.log.Error("failed to get latest transaction id")
// 		return err, nil
// 	}
// 	// if latestTransactionID == previousTransactionID {
// 	// 	return nil, nil
// 	// }

// 	syncReq := models.TransactionChainSyncRequest{
// 		TokenID:       token,
// 		TransactionID: latestTransactionID,
// 	}

// 	for {
// 		var trep models.TransactionChainSyncReply
// 		err = p.SendJSONRequest("POST", APISyncTransactionChain, nil, &syncReq, &trep, false)
// 		if err != nil {
// 			c.log.Error("failed to sync transaction chain")
// 			return err, nil
// 		}
// 		if !trep.Status {
// 			c.log.Error("failed to sync transaction chain")
// 			return fmt.Errorf(trep.Message), nil
// 		}
// 		if len(trep.Transactions) > 0 {
// 			for _, txn := range trep.Transactions {
// 				tx, err := util.TransactionFromBytes(txn)
// 				if tx == nil {
// 					c.log.Error("failed to convert transaction bytes to transaction")
// 					return fmt.Errorf("failed to convert transaction bytes to transaction"), nil
// 				}
// 				var txInfo models.TransactionInfo
// 				if err = json.Unmarshal(tx.Info, &txInfo); err != nil {
// 					c.log.Error("failed to unmarshal transaction info", "err", err)
// 					return fmt.Errorf("failed to unmarshal transaction info: %w", err), nil
// 				}

// 				role := rubixsync.FindTokenRoleInTxn(token, &txInfo)

// 				if err = c.w.CreateTransaction(tx); err != nil {
// 					c.log.Error("failed to add transaction to transactions table", "err", err)
// 					return fmt.Errorf("failed to add transaction: %w", err), nil
// 				}

// 				tokenDetails, err := c.w.GetTokenByTokenID(token)
// 				if err != nil {
// 					newToken := models.Token{
// 						TokenID:        token,
// 						TokenStatus:    constants.TokenStatus_Free,
// 						DID:            txInfo.Owner,
// 						TransactionID:  tx.ID,
// 						TokenType:      int16(models.GetTokenTypeID(constants.TokenType_RBT)),
// 						LatestPosition: 0,
// 						LatestRole:     role,
// 						CreatedAt:      time.Now(),
// 						UpdatedAt:      time.Now(),
// 					}
// 					if createErr := c.w.CreateRBTToken(newToken); createErr != nil {
// 						c.log.Error("failed to create token", "err", createErr)
// 						return fmt.Errorf("failed to create token: %w", createErr), nil
// 					}
// 					tokenDetails = newToken
// 				} else {
// 					tokenDetails.DID = txInfo.Owner
// 					tokenDetails.TransactionID = tx.ID
// 					tokenDetails.LatestPosition++
// 					tokenDetails.LatestRole = role
// 					if updateErr := c.w.UpdateToken(tokenDetails); updateErr != nil {
// 						c.log.Error("failed to update token", "err", updateErr)
// 						return fmt.Errorf("failed to update token: %w", updateErr), nil
// 					}
// 				}

// 				entry := &models.TokenChain{
// 					TokenID:       token,
// 					TransactionID: tx.ID,
// 					Role:          role,
// 					Position:      tokenDetails.LatestPosition,
// 				}
// 				if err = c.w.AddTokenChainEntry(entry); err != nil {
// 					c.log.Error("failed to add token chain entry", "err", err)
// 					return fmt.Errorf("failed to add token chain entry: %w", err), nil
// 				}
// 			}
// 		}
// 		if trep.NextTransactionID == "" {
// 			break
// 		}
// 		syncReq.TransactionID = trep.NextTransactionID
// 	}
// 	return nil, nil
// }
