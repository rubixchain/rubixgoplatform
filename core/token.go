package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	block "github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	parts "github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const defaultBatchSize = 500                             // Tweak according to RAM/network
const delayInPublshingTxnHistory = 30 * time.Millisecond // Throttle interval, tune further
const delayInPublishingTCDetails = 2 * time.Second

const subscriberBufferSize = 1000 // process up to this many idle batches
const workerCount = 8             // Tune according to hardware/network
const MaxNumberOfChildTokensAllowed = 2
const wholeTokenValue = 1.0

// const MaxPossiblePartTokenNumber = 1332

type TokenPublish struct {
	Token string `json:"token"`
}

type TCBSyncRequest struct {
	Token       string `json:"token"`
	TokenType   int    `json:"token_type"`
	BlockID     string `json:"block_id"`
	BlockHeight uint64 `json:"block_height"`
}

type TCBSyncReply struct {
	Status      bool     `json:"status"`
	Message     string   `json:"message"`
	NextBlockID string   `json:"next_block_id"`
	TCBlock     [][]byte `json:"tc_block"`
}

type TCBSyncGenesisAndLatestBlockReply struct {
	Status       bool     `json:"status"`
	Message      string   `json:"message"`
	TCBlocks     [][]byte `json:"tc_blocks"`
	GenesisBlock []byte   `json:"tc_genesis_block"`
	LatestBlock  []byte   `json:"tc_latest_block"`
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

type ReceivedBlock struct {
	GenesisBlock *block.Block `json:"genesis_block"`
	LatestBlock  *block.Block `json:"latest_block"`
}

type PubSubEnvelope struct {
	Type string          `json:"type"` // "token" or "txn"
	Data json.RawMessage `json:"data"`
}

func (c *Core) SetupToken() {
	c.l.AddRoute(APISyncTokenChain, "POST", c.syncTokenChain)
	c.l.AddRoute(APISyncGenesisAndLatestBlock, "POST", c.syncGenesisAndLatestBlock)
	c.l.AddRoute(APIUpdateStatus, "PUT", c.updateStatus)
	c.l.AddRoute(APIGetTokenStatus, "GET", c.getTokenStatus)
	c.l.AddRoute(setup.APIRecoverLostTokens, "POST", c.recoverLostTokensHandler)
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
				Status: t.TokenStatus,
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

func (c *Core) GetAccountInfo(did string) (model.DIDAccountInfo, error) {
	wt, err := c.w.GetAllTokens(did)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get tokens", "err", err)
		return model.DIDAccountInfo{}, fmt.Errorf("failed to get tokens")
	}
	info := model.DIDAccountInfo{
		DID: did,
	}
	for _, t := range wt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.RBTAmount = info.RBTAmount + t.TokenValue
			info.RBTAmount = floatPrecision(info.RBTAmount, MaxDecimalPlaces)
		case wallet.TokenIsLocked:
			info.LockedRBT = info.LockedRBT + t.TokenValue
			info.LockedRBT = floatPrecision(info.LockedRBT, MaxDecimalPlaces)
		case wallet.TokenIsPledged:
			info.PledgedRBT = info.PledgedRBT + t.TokenValue
			info.PledgedRBT = floatPrecision(info.PledgedRBT, MaxDecimalPlaces)
		case wallet.TokenIsPinnedAsService:
			info.PinnedRBT = info.PinnedRBT + t.TokenValue
			info.PinnedRBT = floatPrecision(info.PinnedRBT, MaxDecimalPlaces)
		}
	}
	return info, nil
}

func (c *Core) GenerateTestTokens(reqID string, num int, did string, startIndex int) {
	err := c.generateTestTokens(reqID, num, did, startIndex)
	br := model.BasicResponse{
		Status:  true,
		Message: "Test tokens generated successfully",
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

// getTokenIDForLocalTestTokens retrieves the token ID for local test tokens based on the
// provided token level and token number.
func (c *Core) getTokenIDForLocalTestTokens(tokenLevel int, tokenNumber int, did string) (string, error) {
	idValue := strconv.Itoa(tokenLevel) + "_" + strconv.Itoa(tokenNumber)
	return idValue, nil
}

func (c *Core) generateTestTokens(reqID string, num int, did string, startIndex int) error {
	if !c.testNet {
		return fmt.Errorf("generate test token is available in test net")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}
	var currentTokenNumber int

	if startIndex >= 0 {
		currentTokenNumber = startIndex
	} else {
		currentTokenNumber, err = c.w.GetLocalTokenNumber()
		if err != nil {
			return fmt.Errorf("failed to get local test token info, err: %v", err)
		}

	}

	var startTokenNumber int
	var finalTokenNumber int

	if currentTokenNumber == 0 {
		startTokenNumber = 1
	} else {
		startTokenNumber = currentTokenNumber
	}

	finalTokenNumber = startTokenNumber + num
	currentTime := time.Now()

	// Each global index maps to (tokenLevel, numInLevel) per TokenMap: level 10000 = TokenMap 0 (56 tokens), 10001 = TokenMap 1 (4300000), etc.
	for globalIndex := startTokenNumber; globalIndex < finalTokenNumber; globalIndex++ {
		tokenLevel, numInLevel, err := token.GetTokenLevelAndNumberForGlobalIndex(globalIndex)
		if err != nil {
			c.log.Error("Failed to get token level and number for global index", "globalIndex", globalIndex, "err", err)
			return err
		}
		id, err := c.getTokenIDForLocalTestTokens(tokenLevel, numInLevel, did)
		if err != nil {
			c.log.Error("Failed to add token to network", "err", err)
			return err
		}
		gb := &block.GenesisBlock{
			// Type: block.TokenGeneratedType,
			Info: []block.GenesisTokenInfo{
				{Token: id, NetworkID: constants.NetworkID_RBT_Local},
			},
		}
		ti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     id,
					TokenType: token.TestTokenType,
				},
			},
		}

		tcb := &block.TokenChainBlock{
			BlockType:    block.TokenGeneratedType,
			TokenOwner:   did,
			GenesisBlock: gb,
			TransInfo:    ti,
			TokenValue:   floatPrecision(1.0, MaxDecimalPlaces),
			Version:      constants.BlockVersion,
			Epoch:        int(currentTime.Unix()),
		}

		ctcb := make(map[string]*block.Block)
		ctcb[id] = nil

		blk := block.CreateNewBlock(ctcb, tcb)

		if blk == nil {
			c.log.Error("Failed to create new token chain block")
			return fmt.Errorf("failed to create new token chain block")
		}
		err = blk.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update did signature", "err", err)
			return fmt.Errorf("failed to update did signature")
		}
		t := &wallet.Token{
			TokenID:     id,
			DID:         did,
			TokenValue:  1,
			TokenStatus: wallet.TokenIsFree,
		}
		err = c.w.CreateTokenBlock(blk)
		if err != nil {
			c.log.Error("Failed to add token chain", "err", err)
			return err
		}

		tokenIDBuffer := bytes.NewBufferString(id)
		tokenHash, err := c.w.Add(tokenIDBuffer, did, wallet.AddFunc, true)
		if err != nil {
			return fmt.Errorf("createTokensAtLevel: failed to add token to ipfs: %v, err: %v", id, err)
		}

		if _, err := c.w.Pin(tokenHash, wallet.OwnerRole, did, "NA", did, "NA", t.TokenValue); err != nil {
			return fmt.Errorf("createChildTokensAtLevel: failed to pin child token: %v, err: %v", id, err)
		}

		err = c.w.CreateToken(t)
		if err != nil {
			c.log.Error("Failed to create token", "err", err)
			return err
		}
		// publish the transaction in the network with topic : rubix_txns
		blockHash, err := blk.GetHash()
		if err != nil {
			blockHash = ""
			c.log.Error("failed to get block hash")
		}
		publishingTxn := &model.PubSubTxnInfo{
			BlockHash:    blockHash,
			BlockType:    block.TokenGeneratedType,
			AssetType:    RBTTokenType,
			PublisherDID: dc.GetDID(),
			TxnBlock:     blk.GetBlock(),
		}

		err = c.publishTxn(publishingTxn)
		if err != nil {
			c.log.Error("Failed to publish txn", "err", err)
			return err
		}
	}

	if err := c.w.SetLocalTokenNumber(finalTokenNumber); err != nil {
		return fmt.Errorf("failed to set local test token number, err: %v", err)
	}

	return nil
}

func (c *Core) syncTokenChain(req *ensweb.Request) *ensweb.Result {
	var tr TCBSyncRequest

	// Parse request
	if err := c.l.ParseJSON(req, &tr); err != nil {
		c.log.Warn("Failed to parse request", "error", err)
		return c.l.RenderJSON(req, &TCBSyncReply{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}
	var tcbr TCBSyncReply
	tcbr.Message = "Sent all blocks"

	// Fetch token blocks
	blks, nextID, err := c.w.GetAllTokenBlocks(tr.Token, tr.TokenType, tr.BlockID)
	if err != nil {
		c.log.Error("Error fetching token blocks", "error", err)
		return c.l.RenderJSON(req, &TCBSyncReply{
			Status:  false,
			Message: "Error fetching token blocks",
		}, http.StatusInternalServerError)
		// blks, nextID, err = c.w.GetAllTokenBlocks(tr.Token, tr.TokenType, "")
		// if err != nil {

		// } else {
		// 	tcbr.Message = "Sent all blocks"
		// }
	}

	c.log.Debug("no.of blocks sending through sync token chain API ", len(blks))
	/* // Handle case where both error occurred and blocks are nil
	if err != nil && blks == nil {
		c.log.Warn("Token blocks missing and error occurred, falling back to role-based logic", "token", tr.Token)
		return c.handleRoleBasedLogic(tr.Token, req)
	} */

	// Handle other errors
	// if err != nil {
	// 	respMsg := "token block not found for token: " + tr.Token + " and block: " + tr.BlockID
	// 	return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: respMsg}, http.StatusInternalServerError)
	// }

	// Success response
	return c.l.RenderJSON(req, &TCBSyncReply{
		Status:      true,
		Message:     tcbr.Message,
		TCBlock:     blks,
		NextBlockID: nextID,
	}, http.StatusOK)
}

// This function fetches all DIDs from the DID table and it publishes RBT tokens, FT Tokens, NFT Tokens and smart contracts tokens in batches with batch size 500 corresponding to each DID.
// After publishing token details it publishes transaction history details also to the same pubsub.
func (c *Core) publishTokenChainDetailsAndTxnHistory() error {
	allDIDs, err := c.w.GetAllDIDs()
	if err != nil {
		c.log.Error("failed to get all DIDs of the folder", "err", err)
		return err
	}

	var totalRBTCount, totalFTCount, totalNFTCount, totalSCCCount int

	// ---- Process tokens in chunks per DID ----
	for _, didStruct := range allDIDs {
		did := didStruct.DID

		// -------------------- RBT TOKENS --------------------
		offset := 0
		for {
			rbtTokens, err := c.w.GetRBTTokensChunk(did, defaultBatchSize, offset)
			if err != nil {
				c.log.Info("Failed to fetch RBT tokens batch", "did", did, "err", err)

			}
			if len(rbtTokens) == 0 {
				break
			}
			offset += len(rbtTokens)

			tokenDetails := c.prepareTokenDetailsForRBT(rbtTokens)
			c.PublishTokenChainDetailsEvent(tokenDetails)
			totalRBTCount += len(tokenDetails)

			c.log.Info("Published RBT batch", "did", did, "batchSize", len(tokenDetails), "offset", offset)
		}

		// -------------------- FT TOKENS --------------------
		offset = 0
		for {
			ftTokens, err := c.w.GetFTTokensChunk(did, defaultBatchSize, offset)
			if err != nil {
				c.log.Error("Failed to fetch FT tokens batch", "did", did, "err", err)
				break
				// return err
			}
			if len(ftTokens) == 0 {
				break
			}
			offset += len(ftTokens)

			tokenDetails := c.prepareTokenDetailsForFT(ftTokens)
			c.PublishTokenChainDetailsEvent(tokenDetails)
			totalFTCount += len(tokenDetails)

			c.log.Info("Published FT batch", "did", did, "batchSize", len(tokenDetails), "offset", offset)
		}

		// -------------------- NFT TOKENS --------------------
		offset = 0
		for {
			nftTokens, err := c.w.GetNFTTokensChunk(did, defaultBatchSize, offset)
			if err != nil {
				c.log.Error("Failed to fetch NFT tokens batch", "did", did, "err", err)
				break
				// return err
			}
			if len(nftTokens) == 0 {
				break
			}
			offset += len(nftTokens)

			tokenDetails := c.prepareTokenDetailsForNFT(nftTokens)
			c.PublishTokenChainDetailsEvent(tokenDetails)
			totalNFTCount += len(tokenDetails)

			c.log.Info("Published NFT batch", "did", did, "batchSize", len(tokenDetails), "offset", offset)
		}

		// -------------------- SMART CONTRACT TOKENS --------------------
		offset = 0
		for {
			scTokens, err := c.w.GetSmartContractTokensChunk(did, defaultBatchSize, offset)
			if err != nil {
				c.log.Error("Failed to fetch SmartContract tokens batch", "did", did, "err", err)
				break
				// return err
			}
			if len(scTokens) == 0 {
				break
			}
			offset += len(scTokens)

			tokenDetails := c.prepareTokenDetailsForSC(scTokens)
			c.PublishTokenChainDetailsEvent(tokenDetails)
			totalSCCCount += len(tokenDetails)

			c.log.Info("Published SmartContract batch", "did", did, "batchSize", len(tokenDetails), "offset", offset)
		}
	}

	// Log the total summary
	totalOverall := totalRBTCount + totalFTCount + totalNFTCount + totalSCCCount
	c.log.Info("=== Token publishing summary ===",
		"RBT", totalRBTCount,
		"FT", totalFTCount,
		"NFT", totalNFTCount,
		"SmartContracts", totalSCCCount,
		"TotalPublished", totalOverall,
	)

	c.log.Info("tokenchain details got published and entering into the publish txn history function")
	c.PublishTransactionHistory()

	return nil
}

func (c *Core) PublishTCDetails() {
	if c.ps == nil || c.peerID == "" {
		c.log.Warn("Cannot publish token chain details: pub-sub or peerID not initialized")
	} else if err := c.publishTokenChainDetailsAndTxnHistory(); err != nil {
		c.log.Error("Failed to publish token chain details on startup", "err", err)
	}
}
func (c *Core) SubscribeTCDetails() {
	if c.ps == nil {
		c.log.Warn("Cannot subscribe to token chain details: pub-sub not initialized")
	} else if err := c.SubscribeToTokenChainDetails(); err != nil {
		c.log.Error("Failed to subscribe to token chain details", "err", err)
	}
}

// This function handles received token details or transaction history details through the pubsub
func (c *Core) SubscribeToTokenChainDetails() error {
	if c.ps == nil {
		c.log.Warn("PubSub not initialized")
		return fmt.Errorf("pubsub not ready")
	}

	// Token event worker pipeline
	eventCh := make(chan model.TokenChainDetailsEvent, subscriberBufferSize)
	// Start workers
	for i := 0; i < workerCount; i++ {
		go c.tokenDetailWorker(eventCh, i)
	}

	return c.ps.SubscribeTopic("token_chain_details", func(peerID, topic string, data []byte) {
		raw := bytes.TrimSpace(data)

		// --------------------------------------------------------------------
		// 0. Drop empty messages
		// --------------------------------------------------------------------
		if len(raw) == 0 {
			c.log.Warn("Empty pubsub payload; skipping")
			return
		}

		// --------------------------------------------------------------------
		// 1. Try Base64 decode (direct decode)
		// --------------------------------------------------------------------
		decoded, err := base64.StdEncoding.DecodeString(string(raw))
		if err == nil && len(decoded) > 0 && (decoded[0] == '{' || decoded[0] == '"') {
			c.log.Warn("PubSub: Base64 payload detected (direct); decoding")
			raw = decoded
		} else {
			// ----------------------------------------------------------------
			// 1b. Try Base64 after trimming surrounding quotes
			// ----------------------------------------------------------------
			trimmed := bytes.Trim(raw, "\"")
			decoded2, err2 := base64.StdEncoding.DecodeString(string(trimmed))
			if err2 == nil && len(decoded2) > 0 && (decoded2[0] == '{' || decoded2[0] == '"') {
				c.log.Warn("PubSub: Base64 payload detected (quoted); decoding")
				raw = decoded2
			}
		}

		// Now raw may be JSON or still junk

		// --------------------------------------------------------------------
		// 2. Reject non-JSON beginnings
		// --------------------------------------------------------------------
		if raw[0] != '{' && raw[0] != '"' {
			c.log.Warn("Non-JSON pubsub payload; dropping", "raw", string(raw))
			return
		}

		// --------------------------------------------------------------------
		// 3. If payload is a JSON string, unwrap it
		// --------------------------------------------------------------------
		if raw[0] == '"' {
			var unwrapped string
			if err := json.Unmarshal(raw, &unwrapped); err != nil {
				c.log.Warn("Payload was a JSON string but could not unwrap; dropping",
					"err", err, "raw", string(raw))
				return
			}

			raw = []byte(unwrapped)

			if len(raw) == 0 || raw[0] != '{' {
				c.log.Warn("Unwrapped payload not a JSON object; dropping", "raw", string(raw))
				return
			}
		}

		// --------------------------------------------------------------------
		// 4. Detect legacy messages (no type field in root)
		// --------------------------------------------------------------------
		if !bytes.Contains(raw, []byte(`"type"`)) {
			var legacy model.TokenChainDetailsEvent
			if err := json.Unmarshal(raw, &legacy); err != nil {
				c.log.Warn("Legacy JSON but not token event; dropping",
					"err", err, "raw", string(raw))
				return
			}

			c.log.Warn("Legacy token event received; processing")
			eventCh <- legacy
			return
		}

		// --------------------------------------------------------------------
		// 5. Envelope-based event (modern format)
		// --------------------------------------------------------------------
		var env PubSubEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			c.log.Error("Invalid envelope JSON", "err", err, "raw", string(raw))
			return
		}

		switch env.Type {

		case "token":
			var event model.TokenChainDetailsEvent
			if err := json.Unmarshal(env.Data, &event); err != nil {
				c.log.Error("Failed to decode token event", "err", err)
				return
			}
			eventCh <- event

		case "txn":
			var txns []model.FullNodeTxnHistoryInfo
			if err := json.Unmarshal(env.Data, &txns); err != nil {
				c.log.Error("Failed to decode txn event", "err", err)
				return
			}
			go c.processIncomingTransactionHistory(txns)

		default:
			c.log.Warn("Unknown envelope type", "type", env.Type)
		}
	})
}

// Dedicated worker for batch processing
func (c *Core) tokenDetailWorker(eventCh <-chan model.TokenChainDetailsEvent, workerNum int) {
	for event := range eventCh {
		c.processReceivedTokenDetails(event)
	}
}

// Once Fullnode receives the tokenchain details in batches, it process those tokenchain details using this function
func (c *Core) processReceivedTokenDetails(event model.TokenChainDetailsEvent) {
	tokenSyncMap := make(map[string][]TokenSyncInfo)

	batchStart := time.Now()
	for _, detail := range event.TokenDetails {
		if detail.PublisherDid == "" {
			errMsg := fmt.Sprintf("PublisherDID is empty for the token: %v, simply skipping it, while processing reeived token details", detail.Token)
			c.log.Error(errMsg)
			continue
		}

		address := event.PublisherPeerID + "." + detail.PublisherDid

		// add publisher to peer did table, if it is alredy NOT there in the PeerDIDTable
		publisherPeerId := c.w.GetPeerID(detail.PublisherDid)
		if publisherPeerId != event.PublisherPeerID {

			publisherDetails := &wallet.DIDPeerMap{
				DID:    detail.PublisherDid,
				PeerID: event.PublisherPeerID,
			}
			err := c.AddPeerDetails(*publisherDetails)
			if err != nil {
				c.log.Error("failed to add publisher info to DB")
			}
		}

		latestBlock := c.w.GetFullNodeLatestTokenBlock(detail.Token, detail.TokenType)
		existingBlockOwnerDID := latestBlock.GetOwner()
		var latestBlockHeight uint64
		var latestBlockID, txnID, latestBlockHash string
		// var latestBlockID string
		var blocks ReceivedBlock
		var genesisBlock *block.Block
		var err error

		if latestBlock != nil {
			latestBlockHeight, err = latestBlock.GetBlockNumber(detail.Token)
			if err != nil {
				c.log.Warn("failed to get the latest Block Height, syncing full tokenchain", "error", err)
				info := &model.FailedToSyncTokenDetailsInfo{
					TokenID:   detail.Token,
					TokenType: detail.TokenType,
					AssetType: detail.AssetType,
					Did:       detail.PublisherDid,
					Reason:    fmt.Sprintf("failed to get the latest Block Height, syncing full tokenchain, error%v", err),
				}

				if err := c.w.AddFailedTokensToTable(info); err != nil {
					c.log.Error("Failed to record failed token sync in DB", "token", detail.Token, "error", err)
				} else {
					c.log.Info("Recorded failed token sync in DB", "token", detail.Token)
				}
				continue
			}
			latestBlockID, err = latestBlock.GetBlockID(detail.Token)
			if err != nil {
				c.log.Warn("failed to get the latest BlockID, syncing full tokenchain", "error", err)
				info := &model.FailedToSyncTokenDetailsInfo{
					TokenID:   detail.Token,
					TokenType: detail.TokenType,
					AssetType: detail.AssetType,
					Did:       detail.PublisherDid,
					Reason:    fmt.Sprintf("failed to get the latest BlockID, syncing full tokenchain, error%v", err),
				}

				if err := c.w.AddFailedTokensToTable(info); err != nil {
					c.log.Error("Failed to record failed token sync in DB", "token", detail.Token, "error", err)
				} else {
					c.log.Info("Recorded failed token sync in DB", "token", detail.Token)
				}
				continue

			}

			// currentOwner = latestBlock.GetOwner()
			txnID = latestBlock.GetTid()
			genesisBlock = c.w.GetFullNodeGenesisTokenBlock(detail.Token, detail.TokenType)
			//if it is a part RBT Token, check how many child tokens exist for its parent token, if there are more than 2 add it in a table
			if detail.AssetType == RBTTokenType {
				if genesisBlock != nil {
					parentTokenID, err := genesisBlock.GetParentDetials(detail.Token)
					if err != nil {
						c.log.Error("failed to get parent tokenID from the genesis block, token", detail.Token)
					}
					//ReadFullNode RBT Table, and count how many children it's parent tokenID is having
					//if it is more than 2 add this parent tokenID along with its child tokens to another table
					childTokens, err := c.w.GetChildTokensFromSyncedRBTTable(parentTokenID)
					if len(childTokens) > MaxNumberOfChildTokensAllowed {
						//add these child tokens and parent tokens into a new fullnode table.
						for _, childToken := range childTokens {
							token := model.FullNodeMultipleChildTokens{
								ParentTokenID: childToken.ParentTokenID,
								ChildTokenID:  childToken.TokenID,
							}
							err := c.w.AddTokenToMultipleParentsTable(&token)
							if err != nil {
								c.log.Error("failed to add a parent token to Fullnode's multiple child token table, parentToken", token.ParentTokenID)
							}

						}
					}

				}
			}

			blocks = ReceivedBlock{
				GenesisBlock: genesisBlock,
				LatestBlock:  latestBlock,
			}

		}

		//collecting all those tokens, for which latestblock is empty or publisher's side tokenchain length is more,
		// Fullnode will use these collected tokens later to sync from the publisher
		if latestBlock == nil || detail.TokenChainLength > latestBlockHeight {
			c.log.Debug("Publisher chain longer or full node need entire chain, queuing for sync", "token", detail.Token, "publisherLength", detail.TokenChainLength, "localHeight", latestBlockHeight, "peerAddr", address, "AssetType", detail.AssetType)
			c.AddTokenContentToPSQL(detail.Token, detail.AssetType)
			tokenSyncMap[address] = append(tokenSyncMap[address], TokenSyncInfo{
				TokenID:    detail.Token,
				TokenType:  detail.TokenType,
				AssetType:  detail.AssetType,
				TokenValue: detail.TokenValue,
			})
			//fullnode has equal number of token chain length compared to publisher, check whether the same block is coming or not
			//If the incoming block from the publisher is different than the block which the fullnode is having, add this token details to double spend tokens table
		} else if detail.TokenChainLength == latestBlockHeight {

			// check if token exists in postgres table, add if doesn't
			err := c.ReadTokenContentFromPSQL(detail.Token, detail.AssetType)
			if err != nil {
				if err := c.AddTokenContentToPSQL(detail.Token, detail.AssetType); err != nil {
					c.log.Error("failed to add token's ipfs content to psql db, err: %v", err)
				}
			}

			if detail.LastBlockID != latestBlockID {
				//Add token into double spend tokens table
				doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
					TokenID:        detail.Token,
					AssetType:      detail.AssetType,
					TokenType:      detail.TokenType,
					PublisherDID:   event.PublisherPeerID,
					ClaimedOwnerI:  existingBlockOwnerDID,
					ClaimedOwnerII: detail.PublisherDid,
					ErrorMessage:   fmt.Sprintf("%s, %s both dids are claiming the same token,dual ownership issue", existingBlockOwnerDID, detail.PublisherDid),
				}
				// store double spent token info in DoubleSpentTokens table, and remove it from respective tokens table
				err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
				if err != nil {
					errMsg := fmt.Sprintf("failed to update double spent token : %v, err: %v", detail.Token, err)
					c.log.Error(errMsg)
				}
				continue

			} else {
				// it should read the sqlite table, if already details exist
				// first read existing token info from the table, if not exist we will add it.
				_, _, _, err := c.ReadTokenFromFullnodeTokensTable(detail.AssetType, detail.Token)
				if err != nil {
					if strings.Contains(err.Error(), "no records found") {
						// add token info to sqlite if not there
						eventData := model.PubSubTxnInfo{
							BlockHash:         latestBlockHash,
							TransactionID:     txnID,
							PublisherDID:      detail.PublisherDid,
							LatestBlockHeight: latestBlockHeight,
							AssetType:         detail.AssetType,
							// TokenValue:        detail.TokenValue,
						}
						//To update the token value first look at the genesis block type, If it is migrated type update the token value as whatever publisher published because,
						// There is no token value in the Mainnet genesis block for migrated tokens.
						//In rest of the genesis blocks case, read token value from the genesis block and update the sqlite table
						if genesisBlock != nil {
							genesisBlockType := genesisBlock.GetTransType()
							if genesisBlockType == block.TokenMigratedType {

								eventData.TokenValue = detail.TokenValue
							} else {
								eventData.TokenValue = genesisBlock.GetTokenValue()
							}
						}

						c.AddTokenToRespectiveTable(detail.Token, existingBlockOwnerDID, blocks, &eventData, wallet.SyncUnrequired)
						continue
					}

					c.log.Error("failed to read token ", detail.Token, "err ", err)
					continue
				}
			}

			//Fullnode already has token chain length more than what publisher is having.
			//In this case, get incoming blockID, check whether it matches the same number's blockID at which it is there at Fullnode side.
		} else {

			// check if token exists in postgres table, add if doesn't
			err := c.ReadTokenContentFromPSQL(detail.Token, detail.AssetType)
			if err != nil {
				if err := c.AddTokenContentToPSQL(detail.Token, detail.AssetType); err != nil {
					c.log.Error("failed to add token's ipfs content to psql db, err: %v", err)
				}
			}

			incomingBlockID := detail.LastBlockID
			publishersLatestBlockNumber := detail.TokenChainLength
			//get Fullnode block whose block number is publisher side latest blockID
			fullnodesideBlockBytes, err := c.w.GetFullNodeTokenBlockByNumber(detail.Token, detail.TokenType, publishersLatestBlockNumber)
			fullnodesideBlock := block.InitBlock(fullnodesideBlockBytes, nil)
			fullnodesideBlockID, err := fullnodesideBlock.GetBlockID(detail.Token)
			if err != nil {
				c.log.Error("failed to get blockID of the fullnode side block", "error", err)
			}
			if incomingBlockID != fullnodesideBlockID {
				//add token to double spend tokens table, with an error saying
				//that both DID1 or DID2 are claimming the same token.

				//Add token into double spend tokens table
				doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
					TokenID:        detail.Token,
					AssetType:      detail.AssetType,
					TokenType:      detail.TokenType,
					PublisherDID:   event.PublisherPeerID,
					ClaimedOwnerI:  existingBlockOwnerDID,
					ClaimedOwnerII: detail.PublisherDid,
					ErrorMessage:   fmt.Sprintf("%s, %s both dids are claiming the same token,dual ownership issue", existingBlockOwnerDID, detail.PublisherDid),
				}
				// store double spent token info in DoubleSpentTokens table, and remove it from respective tokens table
				err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
				if err != nil {
					errMsg := fmt.Sprintf("failed to update double spent token : %v, err: %v", detail.Token, err)
					c.log.Error(errMsg)
				}
				continue

			} else {
				//If incoming blkID matches with the fullnodeside blkID and fullnode already has
				// bigger length tokenchain. so it doesn't need any sync from the publisher, fullnode checks whether token details are there in sqlite table, add if not exist.

				// first read existing token info from the table, if not exist we will add it.
				_, _, _, err := c.ReadTokenFromFullnodeTokensTable(detail.AssetType, detail.Token)
				if err != nil {
					if strings.Contains(err.Error(), "no records found") {
						// add token info to sqlite if not there
						eventData := model.PubSubTxnInfo{
							BlockHash:         latestBlockHash,
							TransactionID:     txnID,
							PublisherDID:      detail.PublisherDid,
							LatestBlockHeight: latestBlockHeight,
							AssetType:         detail.AssetType,
							// TokenValue:        detail.TokenValue,
						}
						//To update the token value first look at the genesis block type, If it is migrated type update the token value as whatever publisher published because,
						// There is no token value in the Mainnet genesis block for migrated tokens.
						//In rest of the genesis blocks case, read token value from the genesis block and update the sqlite table
						if genesisBlock != nil {
							genesisBlockType := genesisBlock.GetTransType()
							if genesisBlockType == block.TokenMigratedType {

								eventData.TokenValue = detail.TokenValue
							} else {
								eventData.TokenValue = genesisBlock.GetTokenValue()
							}
						}

						c.AddTokenToRespectiveTable(detail.Token, existingBlockOwnerDID, blocks, &eventData, wallet.SyncUnrequired)
						continue
					}

					c.log.Error("failed to read token ", detail.Token, "err ", err)
					continue
				}
				continue
			}

			// 	latestBlockHash, err := latestBlock.GetHash()
			// 	if err != nil {
			// 		c.log.Error("failed to get latest block hash for the token", detail.Token)
			// 	}
			// 	currentOwner := latestBlock.GetOwner()
			// 	txnID := latestBlock.GetTid()
			// 	genesisBlock := c.w.GetFullNodeGenesisTokenBlock(detail.Token, detail.TokenType)
			// 	blocks := ReceivedBlock{
			// 		GenesisBlock: genesisBlock,
			// 		LatestBlock:  latestBlock,
			// 	}
			// 	// first read existing token info from the table
			// 	_, existingBlockHash, existingOwnerDID, err := c.ReadTokenFromFullnodeTokensTable(detail.AssetType, detail.Token)
			// 	if err != nil {
			// 		if strings.Contains(err.Error(), "no records found") {
			// 			// add token info to sqlite if not there
			// 			eventData := model.PubSubTxnInfo{
			// 				BlockHash:         latestBlockHash,
			// 				TransactionID:     txnID,
			// 				PublisherDID:      detail.PublisherDid,
			// 				LatestBlockHeight: latestBlockHeight,
			// 				AssetType:         detail.AssetType,
			// 				// TokenValue:        detail.TokenValue,
			// 			}
			// 			//To update the token value first look at the genesis block type, If it is migrated type update the token value as whatever publisher published because,
			// 			// There is no token value in the Mainnet genesis block for migrated tokens.
			// 			//In rest of the genesis blocks case, read token value from the genesis block and update the sqlite table
			// 			if genesisBlock != nil {
			// 				genesisBlockType := genesisBlock.GetTransType()
			// 				if genesisBlockType == block.TokenMigratedType {

			// 					eventData.TokenValue = detail.TokenValue
			// 				} else {
			// 					eventData.TokenValue = genesisBlock.GetTokenValue()
			// 				}
			// 			}

			// 			c.AddTokenToRespectiveTable(detail.Token, currentOwner, blocks, &eventData, wallet.SyncUnrequired)
			// 			continue
			// 		}

			// 		c.log.Error("failed to read token ", detail.Token, "err ", err)
			// 		continue
			// 	}
			// 	// if latestBlockHeight == detail.TokenChainLength {
			// 	// 	if latestBlockHash != existingBlockHash || currentOwner != existingOwnerDID {
			// 	// 		// TODO : Challenger node should verify the correct owner and correct block and add the correct info
			// 	// 		errMsg := fmt.Sprintf("double spending the token %v, eixting owner : %v, and incoming owner : %v", detail.Token, existingOwnerDID, currentOwner)
			// 	// 		c.log.Error(errMsg)
			// 	// 		// add token to doublespent tokens table
			// 	// 		doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
			// 	// 			TokenID:        detail.Token,
			// 	// 			AssetType:      detail.AssetType,
			// 	// 			TokenType:      detail.TokenType,
			// 	// 			PublisherDID:   event.PublisherPeerID,
			// 	// 			ClaimedOwnerI:  existingOwnerDID,
			// 	// 			ClaimedOwnerII: currentOwner,
			// 	// 		}
			// 	// 		// store double spent token info in DoubleSpentTokens table
			// 	// 		// and remove it from respective tokens table
			// 	// 		err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
			// 	// 		if err != nil {
			// 	// 			errMsg := fmt.Sprintf("failed to update double spent token : %v, err: %v", detail.Token, err)
			// 	// 			c.log.Error(errMsg)
			// 	// 		}
			// 	// 		continue

			// 	// 	}

			// 	// }

			// 	eventData := model.PubSubTxnInfo{
			// 		BlockHash:         latestBlockHash,
			// 		TransactionID:     txnID,
			// 		PublisherDID:      detail.PublisherDid,
			// 		LatestBlockHeight: latestBlockHeight,
			// 		AssetType:         detail.AssetType,
			// 		// TokenValue:        detail.TokenValue,
			// 	}
			// 	// when latestBlockHeight != existingBlockHeight OR (latestBlockHeight == existingBlockHeight && blockhashes also matches,
			// 	//  sqlite table should get updated with the values which are derived from the latest block.
			// 	//To update the token value first look at the genesis block type, If it is migrated type update the token value as whatever publisher published because,
			// 	// There is no token value in the Mainnet genesis block for migrated tokens.
			// 	//In rest of the genesis blocks case, read token value from the genesis block and update the sqlite table
			// 	if genesisBlock != nil {
			// 		genesisBlockType := genesisBlock.GetTransType()
			// 		if genesisBlockType == block.TokenMigratedType {

			// 			eventData.TokenValue = detail.TokenValue
			// 		} else {
			// 			eventData.TokenValue = genesisBlock.GetTokenValue()
			// 		}
			// 	}

			// 	c.AddTokenToRespectiveTable(detail.Token, currentOwner, blocks, &eventData, wallet.SyncUnrequired)
			// }
		}

		var wg sync.WaitGroup

		//In the case where tokensyncmap is not nil at last read the fullnode's token table if token value is 0, update it with pubsub token value
		for addr, tokens := range tokenSyncMap {
			_, did, ok := util.ParseAddress(addr)
			if !ok {
				c.log.Error("invalid address: %v", addr)
			}
			for _, token := range tokens {
				wg.Add(1)
				go func(addr string, token TokenSyncInfo) {
					defer wg.Done()

					const maxRetries = 3
					const retryDelay = 2 * time.Second

					var peer *ipfsport.Peer
					var err error

					for attempt := 1; attempt <= maxRetries; attempt++ {
						peer, err = c.getPeer(addr)
						if err == nil && peer != nil {
							break
						}
						c.log.Warn("Failed to open peer connection, retrying...",
							"peer", addr,
							"attempt", attempt,
							"error", err)
						time.Sleep(retryDelay)
					}

					if peer == nil || err != nil {
						c.log.Error("Failed to open peer after retries", "peer", addr, "error", err)

						info := &model.FailedToSyncTokenDetailsInfo{
							TokenID:   token.TokenID,
							TokenType: token.TokenType,
							AssetType: token.AssetType,
							Did:       did,
							Reason:    fmt.Sprintf("Failed to open peer after retries,peer%v, error%v", addr, err),
						}

						if err := c.w.AddFailedTokensToTable(info); err != nil {
							c.log.Error("Failed to record failed token sync in DB", "token", token.TokenID, "error", err)
						} else {
							c.log.Info("Recorded failed token sync in DB", "token", token.TokenID)
						}
						return
					}

					defer peer.Close()

					if err := c.SyncFullTokenChainForFullNode(peer, token); err != nil {
						handled, _ := c.HandleSyncErrorAsDoubleSpent(err, detail.Token, detail.AssetType, detail.TokenType, detail.PublisherDid, existingBlockOwnerDID, detail.PublisherDid, fmt.Sprintf("%s, %s both dids are claiming the same token,dual ownership issue", existingBlockOwnerDID, detail.PublisherDid))
						if handled {
							// double-spend case already logged and stored in HandleSyncErrorAsDoubleSpent
						} else {
							c.log.Error("Failed to sync chain", "token", token.TokenID, "err", err)
							info := &model.FailedToSyncTokenDetailsInfo{
								TokenID:   token.TokenID,
								TokenType: token.TokenType,
								AssetType: token.AssetType,
								Did:       did,
								Reason:    fmt.Sprintf("failed to sync chain,err: %v", err),
							}

							if err := c.w.AddFailedTokensToTable(info); err != nil {
								c.log.Error("Failed to record failed token sync in DB", "token", token.TokenID, "error", err)
							} else {
								c.log.Info("Recorded failed token sync in DB", "token", token.TokenID)
							}
						}
					}
					//if it is a part RBT Token, check how many child tokens exist for its parent token, if there are more than 2 add it in a table
					if detail.AssetType == RBTTokenType {
						if genesisBlock != nil {
							parentTokenID, err := genesisBlock.GetParentDetials(detail.Token)
							if err != nil {
								c.log.Error("failed to get parent tokenID from the genesis block, token", detail.Token)
							}
							//ReadFullNode RBT Table, and count how many children it's parent tokenID is having
							//if it is more than 2 add this parent tokenID along with its child tokens to another table
							childTokens, err := c.w.GetChildTokensFromSyncedRBTTable(parentTokenID)
							if len(childTokens) > MaxNumberOfChildTokensAllowed {
								//add these child tokens and parent tokens into a new fullnode table.
								for _, childToken := range childTokens {
									token := model.FullNodeMultipleChildTokens{
										ParentTokenID: childToken.ParentTokenID,
										ChildTokenID:  childToken.TokenID,
									}
									err := c.w.AddTokenToMultipleParentsTable(&token)
									if err != nil {
										c.log.Error("failed to add a parent token to Fullnode's multiple child token table, parentToken", token.ParentTokenID)
									}

								}
							}

						}
					}

				}(addr, token)
			}
		}

		wg.Wait()

		// End timer after all syncs are done
		batchDuration := time.Since(batchStart)

		// Log batch sync time and details
		c.log.Info("Completed token chain sync batch ",
			"batch number", event.BatchNumber,
			"num_peers to connect and sync tokenchain", len(tokenSyncMap),
			"**********number_of_tokens_received_in the batch******** ", len(event.TokenDetails),
			"total_duration_in_minutes", batchDuration.Minutes())

	}
}

// processRole handles specific roles (as integers) and returns a message
func (c *Core) processRole(role int) string {
	roleMessages := map[int]string{
		wallet.OwnerRole:                  "Token chain block does not exist, the pinned role is owner, so this can be a double spend attempt",
		wallet.QuorumRole:                 "Token chain block does not exist, the pinned role is QuorumRole",
		wallet.PrevSenderRole:             "Token chain block does not exist, the pinned role is PrevSenderRole",
		wallet.ReceiverRole:               "Token chain block does not exist, the pinned role is ReceiverRole",
		wallet.ParentTokenLockRole:        "Token chain block does not exist, the pinned role is ParentTokenLockRole",
		wallet.DIDRole:                    "Token chain block does not exist, the pinned role is DIDRole",
		wallet.StakingRole:                "Token chain block does not exist, the pinned role is StakingRole",
		wallet.PledgingRole:               "Token chain block does not exist, the pinned role is PledgingRole",
		wallet.QuorumPinRole:              "Token chain block does not exist, the pinned role is QuorumPinRole",
		wallet.QuorumUnpinRole:            "Token chain block does not exist, the pinned role is QuorumUnpinRole",
		wallet.ParentTokenPinByQuorumRole: "Token chain block does not exist, the pinned role is ParentTokenPinByQuorumRole",
		wallet.PinningRole:                "Token chain block does not exist, the pinned role is PinningRole",
	}

	if message, exists := roleMessages[role]; exists {
		c.log.Info("Processing role", "role", role)
		return message
	}

	c.log.Warn("Unhandled role encountered", "role", role)
	return ""
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

// recoverLostTokensHandler handles P2P requests for token recovery
func (c *Core) recoverLostTokensHandler(req *ensweb.Request) *ensweb.Result {
	var recoveryReq struct {
		SenderDID     string `json:"sender_did"`
		TransactionID string `json:"transaction_id"`
	}

	// Parse request
	if err := c.l.ParseJSON(req, &recoveryReq); err != nil {
		c.log.Warn("Failed to parse recovery request", "error", err)
		return c.l.RenderJSON(req, &struct {
			Status  bool   `json:"status"`
			Message string `json:"message"`
		}{
			Status:  false,
			Message: "Failed to parse request",
		}, http.StatusBadRequest)
	}

	c.log.Info("Received P2P token recovery request",
		"sender_did", recoveryReq.SenderDID,
		"transaction_id", recoveryReq.TransactionID)

	// Perform the recovery
	result, err := c.RecoverLostTokens(recoveryReq.SenderDID, recoveryReq.TransactionID)
	if err != nil {
		c.log.Error("Token recovery failed",
			"sender_did", recoveryReq.SenderDID,
			"transaction_id", recoveryReq.TransactionID,
			"error", err)
		return c.l.RenderJSON(req, &struct {
			Status  bool   `json:"status"`
			Message string `json:"message"`
		}{
			Status:  false,
			Message: err.Error(),
		}, http.StatusOK)
	}

	// Return success response
	return c.l.RenderJSON(req, &struct {
		Status  bool                 `json:"status"`
		Message string               `json:"message"`
		Result  *TokenRecoveryResult `json:"result"`
	}{
		Status:  true,
		Message: "Token recovery successful",
		Result:  result,
	}, http.StatusOK)
}

func (c *Core) UpdateTokenStatus(updateReq *model.UpdateTokenStatusReq) error {
	p, err := c.getPeer(updateReq.DID)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return err
	}
	defer p.Close()
	var updateResp model.BasicResponse

	err = p.SendJSONRequest("PUT", APIUpdateStatus, nil, &updateReq, &updateResp, false)
	if !updateResp.Status {
		c.log.Error("Failed to update status", "err", err)
		return fmt.Errorf(updateResp.Message)
	}
	return nil
}

func (c *Core) GetTokenStatus(getTokenStatusReq *model.GetTokenStatusReq) (model.TokenStatusResponse, error) {
	var resp model.TokenStatusResponse
	p, err := c.getPeer(getTokenStatusReq.DID)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return resp, err
	}
	defer p.Close()
	err = p.SendJSONRequest("GET", APIGetTokenStatus, nil, &getTokenStatusReq, &resp, false)
	if err != nil {
		c.log.Error("Failed to get status", "err", err)
		return resp, err
	}
	return resp, nil
}

func (c *Core) syncTokenChainFrom(p *ipfsport.Peer, pblkID string, token string, tokenType int) (error, *TCBSyncReply) {
	// p, err := c.getPeer(address)
	// if err != nil {
	// 	c.log.Error("Failed to get peer", "err", err)
	// 	return err
	// }
	// defer p.Close()

	// Use token sync manager to prevent race conditions
	if !c.tokenSyncManager.AcquireSyncLock(token) {
		// Another sync is in progress, wait for it to complete
		c.log.Debug("Token sync already in progress, waiting", "token", token)
		if err := c.tokenSyncManager.WaitForSync(token, 30*time.Second); err != nil {
			return err, nil
		}
		// Check if we still need to sync after waiting
		blk := c.w.GetLatestTokenBlock(token, tokenType)
		if blk != nil {
			blkID, _ := blk.GetBlockID(token)
			if blkID == pblkID {
				return nil, nil // Already synced
			}
		}
	}
	defer c.tokenSyncManager.ReleaseSyncLock(token)

	var err error
	blk := c.w.GetLatestTokenBlock(token, tokenType)
	if blk != nil {
		_, err = blk.GetBlockNumber(token)
		if err != nil {
			c.log.Error("Failed to get block number while syncing", "err", err)
			return err, nil
		}
	}
	blkID := ""
	if blk != nil {
		blkID, err = blk.GetBlockID(token)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return err, nil
		}
		if blkID == pblkID {
			return nil, nil
		}
		_, err = blk.GetBlockNumber(token)
		if err != nil {
			c.log.Error("invalid block, failed to get block number")
			return err, nil
		}
	}
	syncReq := TCBSyncRequest{
		Token:     token,
		TokenType: tokenType,
		BlockID:   blkID,
	}

	// if tokenType == c.TokenType(RBTString) || tokenType == c.TokenType(PartString) {
	// 	syncReq.BlockHeight = blkHeight
	// 	// sync only latest blcok of the token chain for the transaction
	// 	err = c.syncGenesisAndLatestBlockFrom(p, syncReq)
	// 	if err != nil {
	// 		c.log.Error("failed to sync latest block, err ", err)
	// 		return err
	// 	}
	// 	// update sync status to incomplete
	// 	err = c.w.UpdateTokenSyncStatus(syncReq.Token, wallet.SyncIncomplete)
	// 	if err != nil {
	// 		if !strings.Contains(err.Error(), "no records found") {
	// 			c.log.Error("failed to update token sync status as incomplete, token ", token)
	// 		}
	// 	}
	// } else {
	// in case of FTs, and NFTs
	for {
		var trep TCBSyncReply
		err = p.SendJSONRequest("POST", APISyncTokenChain, nil, &syncReq, &trep, false)
		//c.log.Debug("syncTokenChainFrom: Sent sync request", "request", syncReq)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			return err, &trep
		}
		//c.log.Debug("syncTokenChainFrom: Received response", "response", trep)
		if !trep.Status {
			c.log.Error("Failed to sync token chain block", "msg", trep.Message)
			return fmt.Errorf(trep.Message), &trep
		}
		if len(trep.TCBlock) > 0 {
			for _, bb := range trep.TCBlock {
				blk := block.InitBlock(bb, nil)
				if blk == nil {
					c.log.Error("Failed to add token chain block, invalid block, sync failed", "err", err)
					return fmt.Errorf("failed to add token chain block, invalid block, sync failed"), &trep
				}
				err = c.w.AddTokenBlock(token, blk)
				if err != nil {
					c.log.Error("Failed to add token chain block, syncing failed", "err", err)
					return err, &trep
				}
			}
		}
		if trep.NextBlockID == "" {
			break
		}
		syncReq.BlockID = trep.NextBlockID
	}
	// }
	return nil, nil
}

// using this function, full node can sync entire token chain of the token
func (c *Core) SyncFullTokenChainForFullNode(p *ipfsport.Peer, tokenSyncInfo TokenSyncInfo) error {
	var err error

	blk := c.w.GetFullNodeLatestTokenBlock(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType)
	if blk != nil {
		_, err = blk.GetBlockNumber(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("Failed to get block number while syncing", "err", err, "token", tokenSyncInfo.TokenID)
			return err
		}
	}

	blkID := ""
	if blk != nil {
		blkID, err = blk.GetBlockID(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("Failed to get block id", "err", err, "token", tokenSyncInfo.TokenID)
			return err
		}

	}

	syncReq := TCBSyncRequest{
		Token:     tokenSyncInfo.TokenID,
		TokenType: tokenSyncInfo.TokenType,
		BlockID:   blkID,
	}

	var syncerLatestBlkID string
	for {
		var trep TCBSyncReply
		err = p.SendJSONRequest("POST", APISyncTokenChain, nil, &syncReq, &trep, false)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err, "token", tokenSyncInfo.TokenID)
			return err
		}
		if !trep.Status {
			c.log.Error("Failed to sync token chain block", "msg", trep.Message, "token", tokenSyncInfo.TokenID)
			return fmt.Errorf("sync failed: %s", trep.Message)
		}

		if strings.Contains(trep.Message, "Sent all blocks") {
			if len(trep.TCBlock) > 0 {
				peerLatestBlk := block.InitBlock(trep.TCBlock[len(trep.TCBlock)-1], nil)
				if peerLatestBlk == nil {
					c.log.Error("Failed to initialize peer's latest block", "token", tokenSyncInfo.TokenID)
					return fmt.Errorf("failed to initialize peer's latest block")
				}
				syncerLatestBlkID, err = peerLatestBlk.GetBlockID(tokenSyncInfo.TokenID)
				if err != nil {
					c.log.Error("Failed to get blockID of synced block", "err", err, "token", tokenSyncInfo.TokenID)
					return err
				}
			}
		}

		for _, bb := range trep.TCBlock {
			blk := block.InitBlock(bb, nil)
			if blk == nil {
				c.log.Error("Failed to add token chain block, invalid block", "token", tokenSyncInfo.TokenID)
				return fmt.Errorf("failed to add token chain block, invalid block")
			}

			latestBlock := c.w.GetFullNodeLatestTokenBlock(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType)

			err = c.ValidateIncomingTokenBlock(*blk, latestBlock, tokenSyncInfo.TokenID, p, tokenSyncInfo.AssetType)
			if err != nil {
				return err
			}

			//if all checks pass, add it to the levelDB.
			err = c.w.AddFullNodeTokenBlock(tokenSyncInfo.TokenID, blk)
			if err != nil {
				c.log.Error("Failed to add token chain block, syncing failed", "err", err, "token", tokenSyncInfo.TokenID)
				return fmt.Errorf("failed to add token chain block, syncing failed: err=%v, token=%s",
					err,
					tokenSyncInfo.TokenID,
				)
			}
		}

		if trep.NextBlockID == "" {
			break
		}
		syncReq.BlockID = trep.NextBlockID
	}

	latestBlock := c.w.GetFullNodeLatestTokenBlock(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType)
	if latestBlock == nil {
		errMsg := fmt.Sprintf("failed to add synced token blocks of token : %v", tokenSyncInfo.TokenID)
		c.log.Error(errMsg)
		return fmt.Errorf(errMsg)
	}

	genesisBlock := c.w.GetFullNodeGenesisTokenBlock(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType)

	if genesisBlock == nil {

		errMsg := fmt.Sprintf("genesis block is NIL even after syncing for the token: %v", tokenSyncInfo.TokenID)
		c.log.Error(errMsg)
		//return error and add this failed to sync token,  to FullNodeFailedToSyncTokens sqlite table
		return fmt.Errorf(errMsg)
	}

	var ownerDid string
	var blockHash, transactionID string
	var latestBlockHeight uint64

	// syncStatus := wallet.SyncCompleted
	latestBlockAfterSync := c.w.GetFullNodeLatestTokenBlock(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType)
	if latestBlockAfterSync != nil {
		ownerDid = latestBlockAfterSync.GetOwner()
		transactionID = latestBlockAfterSync.GetTid()
		blockHash, err = latestBlockAfterSync.GetHash()
		// latestBlockID, err = latestBlockAfterSync.GetBlockID(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("failed to get latest block hash", "token: ", tokenSyncInfo.TokenID)
		}
		latestBlockID, err := latestBlockAfterSync.GetBlockID(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("failed to get latest blockID after syncing full tokenchain", "token: ", tokenSyncInfo.TokenID)
		}
		latestBlockHeight, err = latestBlockAfterSync.GetBlockNumber(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("failed to get latest block height after syncing full tokenchain", "token: ", tokenSyncInfo.TokenID)
		}
		if syncerLatestBlkID != latestBlockID {
			c.log.Error("token is not synced completely, latest blockIDs are not matching at syncer side and peer's side")
			return fmt.Errorf("token is not synced completely, latest blockIDs are not matching at syncer side and peer's side")

		} else { // meaning sync completed properly
			event := &model.PubSubTxnInfo{
				BlockHash:         blockHash,
				TransactionID:     transactionID,
				AssetType:         tokenSyncInfo.AssetType,
				LatestBlockHeight: latestBlockHeight,
				PublisherDID:      p.GetPeerDID(),
				TokenValue:        tokenSyncInfo.TokenValue,
			}
			syncStatus := wallet.SyncCompleted
			if genesisBlock != nil {
				blocks := ReceivedBlock{
					GenesisBlock: genesisBlock,
					LatestBlock:  latestBlockAfterSync,
				}
				//add synced tokens to respective sqlite tables

				err = c.AddTokenToRespectiveTable(tokenSyncInfo.TokenID, ownerDid, blocks, event, syncStatus)
				if err != nil {
					c.log.Info("Failed to add token details to respective tables", "token", tokenSyncInfo.TokenID, "err", err)
					// return err
				}
			}
			//If sync is completed remove those tokens from the FullNodeFailedToSyncTokens table of the fullnode.
			if syncStatus == wallet.SyncCompleted {

				err := c.w.DeleteFailedToSyncTokenFromTable(tokenSyncInfo.TokenID)
				if err != nil {
					c.log.Error("Failed to Delete token details from the FullNodeFailedToSyncTokens table", "token", tokenSyncInfo.TokenID, "err", err)
					return err
				}
			}

		}

	} else {
		c.log.Error("latest block after sync is still nil ")
		return fmt.Errorf("latest block after sync is still nil, token%s", tokenSyncInfo.TokenID)
	}

	return nil
}

func (c *Core) syncMissingBlocks(p *ipfsport.Peer, tokenSyncInfo TokenSyncInfo) error {
	// read the level db and check the block number sequence and return the block numbers that needs to be synced
	// if all blocks are synced then mark the token sync status as completed
	minMissingBlockId, maxMissingblockId, err := c.GetMissingBlockSequence(tokenSyncInfo)
	if err != nil {
		c.log.Error("failed to fetch missing block sequence", "error", err)
		return err
	}

	if minMissingBlockId == "" && maxMissingblockId == "" {
		c.log.Debug("token chain is completely synced")
		// update token sync status
		err = c.w.UpdateTokenSyncStatus(tokenSyncInfo.TokenID, wallet.SyncCompleted)
		if err != nil {
			c.log.Error("failed to update token sync status for token ", tokenSyncInfo.TokenID)
			return err
		}
		return nil
	}
	//prepare sync request
	syncReq := TCBSyncRequest{
		Token:     tokenSyncInfo.TokenID,
		TokenType: int(tokenSyncInfo.TokenType),
		BlockID:   minMissingBlockId,
	}

	for {
		var trep TCBSyncReply
		err := p.SendJSONRequest("POST", APISyncTokenChain, nil, &syncReq, &trep, false)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
		}
		if !trep.Status {
			c.log.Error("Failed to sync token chain block", "msg", trep.Message)
		}
		for _, bb := range trep.TCBlock {
			blk := block.InitBlock(bb, nil)
			if blk == nil {
				c.log.Error("Failed to initiate token chain block, invalid block, sync failed", "err", err)
			}
			blkId, err := blk.GetBlockID(tokenSyncInfo.TokenID)
			if err != nil {
				c.log.Error("failed to get block Id ")
			}
			if blkId == maxMissingblockId {
				break
			}
			err = c.w.AddMissingTokenBlock(syncReq.Token, blk)
			if err != nil {
				c.log.Error("Failed to add token chain block, syncing failed", "err", err)
				return err
			}
		}
		if trep.NextBlockID == maxMissingblockId || trep.NextBlockID == "" {
			break
		}
		syncReq.BlockID = trep.NextBlockID
	}
	return nil
}

func (c *Core) syncMissingBlocksOfTokenChains(tokenSyncMap map[string][]TokenSyncInfo) {
	// sync sequencially for each peer
	for peerAddr, tokenSyncInfo := range tokenSyncMap {
		p, err := c.getPeer(peerAddr)
		if err != nil {
			c.log.Error("failed to sync full token chain, failed to open peer connection with peer ", peerAddr)
			return
		}
		defer p.Close()
		// start syncing all tokens in queue
		for _, tokenToSync := range tokenSyncInfo {
			c.log.Debug("syncing token: " + tokenToSync.TokenID)
			err := c.syncMissingBlocks(p, tokenToSync)
			if err != nil {
				c.log.Error("failed to sync token chain for token ", tokenToSync.TokenID, "error", err)
				// update sync status to incomplete
				_ = c.w.UpdateTokenSyncStatus(tokenToSync.TokenID, wallet.SyncIncomplete)
				continue
			}
			// update sync status to completed
			err = c.w.UpdateTokenSyncStatus(tokenToSync.TokenID, wallet.SyncCompleted)
			if err != nil {
				c.log.Error("failed to update sync status after sync completed, token ", tokenToSync.TokenID)
				continue
			}
			c.log.Debug("sync completed, updated sync status, token: " + tokenToSync.TokenID)
		}
	}

}
func (c *Core) syncFullTokenChains(tokenSyncMap map[string][]TokenSyncInfo) {
	start := time.Now()
	// sync sequencially for each peer
	for peerAddr, tokenSyncInfo := range tokenSyncMap {
		p, err := c.getPeer(peerAddr)
		if err != nil {
			c.log.Error("failed to sync full token chain, failed to open peer connection with peer ", peerAddr)
			return
		}
		defer p.Close()
		// start syncing all tokens in queue
		for _, tokenToSync := range tokenSyncInfo {
			c.log.Debug("syncing token: " + tokenToSync.TokenID)
			err := c.SyncFullTokenChainForFullNode(p, tokenToSync)
			if err != nil {
				c.log.Error("failed to sync token chain for token ", tokenToSync.TokenID, "error", err)
				// // update sync status to incomplete
				// _ = c.w.UpdateTokenSyncStatus(tokenToSync.TokenID, wallet.SyncIncomplete)
				continue
			}
			//Add a logic to write the token details into tokenstorage table

			// // update sync status to completed
			// err = c.w.UpdateTokenSyncStatus(tokenToSync.TokenID, wallet.SyncCompleted)
			// if err != nil {
			// 	c.log.Error("failed to update sync status after sync completed, token ", tokenToSync.TokenID, "error: ", err)
			// 	continue
			// }
			c.log.Debug("sync completed, token: " + tokenToSync.TokenID)
		}
	}
	timeTaken := time.Since(start)

	c.log.Info("Time taken to sync all tokens is: ", timeTaken)

}

func (c *Core) syncGenesisAndLatestBlock(req *ensweb.Request) *ensweb.Result {
	var tr TCBSyncRequest

	err := c.l.ParseJSON(req, &tr)
	if err != nil {
		c.log.Error("failed to parse latest block sync request")
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: "Failed to parse sync request"}, http.StatusOK)
	}

	c.log.Debug("received sync request", tr)
	trep := TCBSyncGenesisAndLatestBlockReply{
		Status:  true,
		Message: "Got latest block",
		// TCBlocks: make([][]byte, 2),
	}

	if tr.BlockID == "" {
		genesisBlock := c.w.GetGenesisTokenBlock(tr.Token, tr.TokenType)
		if genesisBlock == nil {
			c.log.Error("genesis block is nil, invalid token chain, failed to share token chain")
			return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: "genesis block is nil, invalid token chain"}, http.StatusOK)
		}
		trep.GenesisBlock = genesisBlock.GetBlock()
		c.log.Debug("adding genesis block bytes for token", tr.Token)
	}

	latestBlock := c.w.GetLatestTokenBlock(tr.Token, tr.TokenType)
	if latestBlock == nil {
		c.log.Error("latest block is nil, invalid token chain, failed to share token chain")
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: "latest block is nil, invalid token chain"}, http.StatusOK)
	}
	latestBlockHeight, err := latestBlock.GetBlockNumber(tr.Token)
	if err != nil {
		c.log.Error("failed to get token chain height, err", err)
		return c.l.RenderJSON(req, &TCBSyncReply{Status: false, Message: "failed to get token chain height" + err.Error()}, http.StatusOK)
	}

	if latestBlockHeight != 0 && latestBlockHeight > tr.BlockHeight {
		trep.LatestBlock = latestBlock.GetBlock()
		c.log.Debug("adding latest block bytes ")
	}
	return c.l.RenderJSON(req, &trep, http.StatusOK)
}

func (c *Core) syncGenesisAndLatestBlockFrom(p *ipfsport.Peer, syncReq TCBSyncRequest) error {
	c.log.Debug("sending sync req ", syncReq)
	var trep TCBSyncGenesisAndLatestBlockReply
	err := p.SendJSONRequest("POST", APISyncGenesisAndLatestBlock, nil, &syncReq, &trep, false)
	if err != nil {
		c.log.Error("Failed to sync genesis and latest token chain block", "err", err)
		return err
	}
	if !trep.Status {
		c.log.Error("Failed to sync genesis and latest token chain block", "msg", trep.Message)
		return fmt.Errorf(trep.Message)
	}

	// add genesis block
	if trep.GenesisBlock != nil {
		fmt.Println("adding genesis block")
		genesisBlock := block.InitBlock(trep.GenesisBlock, nil)
		if genesisBlock == nil {
			c.log.Error("Failed to initiate genesis block, invalid block, sync failed", "err", err)
			return fmt.Errorf("failed to initiate genesis block, invalid block, sync failed")
		}
		err = c.w.AddTokenBlock(syncReq.Token, genesisBlock) /// to work on this
		if err != nil {
			c.log.Error("Failed to add genesis block, syncing failed", "err", err)
			return err
		}
	}
	// add latest block
	if trep.LatestBlock != nil {
		fmt.Println("adding latest block")
		latestBlock := block.InitBlock(trep.LatestBlock, nil)
		if latestBlock == nil {
			c.log.Error("Failed to initiate latest block, invalid block, sync failed", "err", err)
			return fmt.Errorf("failed to initiate latest block, invalid block, sync failed")
		}
		err = c.w.AddTokenBlock(syncReq.Token, latestBlock) /// to work on this
		if err != nil {
			c.log.Error("Failed to add latest block, syncing failed", "err", err)
			return err
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

// func (c *Core) tokenStatusCallback(peerID string, topic string, data []byte) {
// 	// c.log.Debug("Recevied token status request")
// 	// var tp TokenPublish
// 	// err := json.Unmarshal(data, &tp)
// 	// if err != nil {
// 	// 	return
// 	// }
// 	// c.log.Debug("Token recevied", "token", tp.Token)
// }

// func (c *Core) GetRequiredTokens(did string, txnAmount float64, txnMode int) ([]wallet.Token, float64, error) {
// 	// Use optimized version for large amounts
// 	if txnAmount > 100 {
// 		c.log.Info("Using optimized token fetch for large amount", "amount", txnAmount)

// 		// Use the wallet's own optimized method
// 		tokens, err := c.w.GetTokensForOptimizedTransfer(did, txnAmount, txnMode)
// 		if err != nil {
// 			return nil, 0, err
// 		}

// 		// Calculate if we have exact amount or need to create change
// 		var totalValue float64
// 		for _, t := range tokens {
// 			totalValue += t.TokenValue
// 		}
// 		totalValue = floatPrecision(totalValue, MaxDecimalPlaces)
// 		remainingAmount := floatPrecision(totalValue-txnAmount, MaxDecimalPlaces)
// 		return tokens, remainingAmount, nil
// 	}

// 	// Original logic for smaller amounts
// 	requiredTokens := make([]wallet.Token, 0)
// 	var remainingAmount float64
// 	wholeValue := int(txnAmount)
// 	//fv := float64(txnAmount)
// 	decimalValue := txnAmount - float64(wholeValue)
// 	decimalValue = floatPrecision(decimalValue, MaxDecimalPlaces)
// 	reqAmt := floatPrecision(txnAmount, MaxDecimalPlaces)
// 	//check if whole value exists
// 	if wholeValue != 0 {
// 		//extract the whole amount part that is the integer value of txn amount
// 		//serach for the required whole amount
// 		wholeTokens, remWhole, err := c.w.GetWholeTokens(did, wholeValue, txnMode)
// 		if err != nil && err.Error() != "no records found" {
// 			c.w.ReleaseTokens(wholeTokens)
// 			c.log.Error("failed to search for whole tokens", "err", err)
// 			return nil, 0.0, err
// 		}

// 		//if whole tokens are found add thgem to the variable required Tokens
// 		if len(wholeTokens) != 0 {
// 			c.log.Debug("found whole tokens in wallet adding them to required tokens list")
// 			requiredTokens = append(requiredTokens, wholeTokens...)
// 			//wholeValue = wholeValue - len(requiredTokens)
// 			reqAmt = reqAmt - float64(len(wholeTokens))
// 			reqAmt = floatPrecision(reqAmt, MaxDecimalPlaces)
// 		}

// 		if (len(wholeTokens) != 0 && remWhole > 0) || (len(wholeTokens) != 0 && remWhole == 0) {
// 			if reqAmt == 0 {
// 				return requiredTokens, remainingAmount, nil
// 			}
// 			c.log.Debug("No more whole token left in wallet , rest of needed amt ", reqAmt)
// 			allPartTokens, err := c.w.GetAllPartTokens(did)
// 			if err != nil {
// 				// In GetAllPartTokens, we first check if there are any part tokens present in
// 				// TokensTable. Now there could be a situation, where there aren't any part tokens
// 				// and it Should not error out, but proceed further. The "no records found" error string
// 				// is usually received from the Read() method the db.
// 				// Hence, in this case, we simply return with whatever values requiredTokens and reqAmt holds
// 				if strings.Contains(err.Error(), "no records found") {
// 					return requiredTokens, reqAmt, nil
// 				}
// 				c.w.ReleaseTokens(wholeTokens)
// 				c.log.Error("failed to lock part tokens", "err", err)
// 				return nil, 0.0, err
// 			}
// 			var sum float64
// 			for _, partToken := range allPartTokens {
// 				sum = sum + partToken.TokenValue
// 				sum = floatPrecision(sum, MaxDecimalPlaces)
// 			}
// 			if sum < reqAmt {
// 				c.w.ReleaseTokens(wholeTokens)
// 				c.log.Error("There are no Whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
// 				return nil, 0.0, fmt.Errorf("there are no whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
// 			}
// 			// Create a slice to store the indices of elements to be removed
// 			var indicesToRemove []int
// 			// Iterate through allPartTokens
// 			defer c.w.ReleaseTokens(allPartTokens)
// 			for i, partToken := range allPartTokens {
// 				// Subtract the partToken value from the txnAmount
// 				// If the transaction amount is less than the partToken.TokenValue, skip
// 				if reqAmt < partToken.TokenValue {
// 					continue
// 				}
// 				reqAmt -= partToken.TokenValue
// 				reqAmt = floatPrecision(reqAmt, MaxDecimalPlaces)
// 				// Add the partToken to the requiredTokens
// 				requiredTokens = append(requiredTokens, partToken)
// 				// Store the index of the element to be removed
// 				indicesToRemove = append(indicesToRemove, i)
// 				// Check if txnAmount goes negative
// 				if reqAmt == 0 {
// 					break
// 				}
// 			}
// 			// Remove elements from allPartTokens using copy
// 			for i, idx := range indicesToRemove {
// 				copy(allPartTokens[idx-i:], allPartTokens[idx-i+1:])
// 			}
// 			allPartTokens = allPartTokens[:len(allPartTokens)-len(indicesToRemove)]
// 			c.w.ReleaseTokens(allPartTokens)

// 			if reqAmt > 0 {
// 				// Add the remaining amount to the remainingAmount variable
// 				remainingAmount += reqAmt
// 				remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
// 			}
// 		}

// 		//if no parts found anf remWhole is also not 0
// 		if len(wholeTokens) == 0 && remWhole > 0 {
// 			c.log.Debug("No whole tokens found. proceeding to get part tokens for txn")

// 			allPartTokens, err := c.w.GetAllPartTokens(did)
// 			if err != nil && err.Error() != "no records found" {
// 				c.log.Error("failed to search for part tokens", "err", err)
// 				return nil, 0.0, err
// 			}
// 			if len(allPartTokens) == 0 {
// 				c.log.Error("No part Tokens found , This wallet is empty", "err", err)
// 				return nil, 0.0, err
// 			}
// 			var sum float64
// 			for _, partToken := range allPartTokens {
// 				sum = sum + partToken.TokenValue
// 			}
// 			if sum < txnAmount {
// 				c.log.Error("There are no Whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
// 				return nil, 0.0, fmt.Errorf("there are no whole tokens and the exisitng decimal balance is not sufficient for the transfer, please use smaller amount")
// 			}
// 			// Create a slice to store the indices of elements to be removed
// 			var indicesToRemove []int
// 			// Iterate through allPartTokens
// 			defer c.w.ReleaseTokens(allPartTokens)
// 			for i, partToken := range allPartTokens {
// 				// Subtract the partToken value from the txnAmount
// 				// If the transaction amount is less than the partToken.TokenValue, skip
// 				if txnAmount < partToken.TokenValue {
// 					continue
// 				}
// 				txnAmount -= partToken.TokenValue
// 				txnAmount = floatPrecision(txnAmount, MaxDecimalPlaces)
// 				// Add the partToken to the requiredTokens
// 				requiredTokens = append(requiredTokens, partToken)
// 				// Store the index of the element to be removed
// 				indicesToRemove = append(indicesToRemove, i)
// 				// Check if txnAmount goes negative
// 				if txnAmount == 0 {
// 					break
// 				}
// 			}
// 			// Remove elements from allPartTokens using copy
// 			for i, idx := range indicesToRemove {
// 				copy(allPartTokens[idx-i:], allPartTokens[idx-i+1:])
// 			}
// 			allPartTokens = allPartTokens[:len(allPartTokens)-len(indicesToRemove)]
// 			c.w.ReleaseTokens(allPartTokens)
// 			if txnAmount > 0 {
// 				// Add the remaining amount to the remainingAmount variable
// 				remainingAmount += txnAmount
// 				remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
// 			}

// 		}
// 	} else {
// 		return make([]wallet.Token, 0), reqAmt, nil
// 	}
// 	defer c.w.ReleaseTokens(requiredTokens)
// 	remainingAmount = floatPrecision(remainingAmount, MaxDecimalPlaces)
// 	return requiredTokens, remainingAmount, nil
// }

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

func (c *Core) UpdatePledgedTokenInfo(tokenstatehash string) error {
	err := c.w.RemoveTokenStateHash(tokenstatehash)
	if err != nil && err.Error() != "no records found" {
		c.log.Error("Failed to get token state hash", "err", err)
	}
	return nil
}

func (c *Core) GetpinnedTokens(did string) ([]wallet.Token, error) {
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

	tokenDetails, err := c.generateTestTokensFaucet(reqID, tokenCount, did)

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

func (c *Core) getFaucetTestTokensID(tokenLevel int, tokenNumber int) (string, error) {
	idStrVal := fmt.Sprintf("%d_%d", tokenLevel, tokenNumber)
	return idStrVal, nil
}

func (c *Core) generateTestTokensFaucet(reqID string, numTokens int, did string) (*token.FaucetToken, error) {

	if !c.testNet {
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
		tokendetail.CurrentTokenNumber += 1

		//If the latest token number to be generated is more than the max token value of previous token, increase the token level
		levelOffset := tokendetail.TokenLevel - constants.FaucetRBT_Level_Offset
		maxTokens := token.TokenMap[levelOffset]
		if tokendetail.CurrentTokenNumber == maxTokens+1 {
			tokendetail.TokenLevel += 1
			tokendetail.CurrentTokenNumber = 1
		}

		id, err := c.getFaucetTestTokensID(tokendetail.TokenLevel, tokendetail.CurrentTokenNumber)
		if err != nil {
			c.log.Error("Failed to get token ID from IPFS", "err", err)
			return &tokendetail, fmt.Errorf("failed to get token ID from IPFS")
		}

		currentTime := time.Now()

		gb := &block.GenesisBlock{
			// Type: block.TokenGeneratedType,
			Info: []block.GenesisTokenInfo{
				{Token: id, NetworkID: constants.NetworkID_RBT_Testnet},
			},
		}
		ti := &block.TransInfo{
			Tokens: []block.TransTokens{
				{
					Token:     id,
					TokenType: token.TestTokenType,
				},
			},
		}

		tcb := &block.TokenChainBlock{
			BlockType:    block.TokenGeneratedType,
			TokenOwner:   did,
			GenesisBlock: gb,
			TransInfo:    ti,
			TokenValue:   floatPrecision(1.0, MaxDecimalPlaces),
			Version:      constants.BlockVersion,
			Epoch:        int(currentTime.Unix()),
		}

		ctcb := make(map[string]*block.Block)
		ctcb[id] = nil

		blk := block.CreateNewBlock(ctcb, tcb)
		//If error comes after adding in IPFS, removing the pin from that token.
		if blk == nil {
			c.log.Error("Failed to create new token chain block")
			tokenHash, err := c.ipfsOps.Add(bytes.NewBufferString(id), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				return &tokendetail, fmt.Errorf("unable to do IPFS Add operation on Token, err: %v", err)
			}
			c.w.UnPin(tokenHash, wallet.OwnerRole, did)
			return &tokendetail, fmt.Errorf("failed to create new token chain block")
		}

		err = blk.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update did signature", "err", err)
			tokenHash, err := c.ipfsOps.Add(bytes.NewBufferString(id), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				return &tokendetail, fmt.Errorf("unable to do IPFS Add operation on Token, err: %v", err)
			}
			c.w.UnPin(tokenHash, wallet.OwnerRole, did)
			return &tokendetail, fmt.Errorf("failed to update did signature")
		}

		t := &wallet.Token{
			TokenID:     id,
			DID:         did,
			TokenValue:  1,
			TokenStatus: wallet.TokenIsFree,
		}

		err = c.w.CreateTokenBlock(blk)
		if err != nil {
			c.log.Error("Failed to add token chain", "err", err)
			tokenHash, err := c.ipfsOps.Add(bytes.NewBufferString(id), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				return &tokendetail, fmt.Errorf("unable to do IPFS Add operation on Token, err: %v", err)
			}
			c.w.UnPin(tokenHash, wallet.OwnerRole, did)
			return &tokendetail, err
		}

		err = c.w.CreateToken(t)
		if err != nil {
			c.log.Error("Failed to create token", "err", err)
			c.w.RemoveTokenChainBlocklatest(t.TokenID, token.TestTokenType)
			tokenHash, err := c.ipfsOps.Add(bytes.NewBufferString(id), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				return &tokendetail, fmt.Errorf("unable to do IPFS Add operation on Token, err: %v", err)
			}
			c.w.UnPin(tokenHash, wallet.OwnerRole, did)
			return &tokendetail, err
		}

		tokendetail.TotalCount += 1
		publishingTxn := &model.PubSubTxnInfo{
			BlockType:    block.TokenGeneratedType,
			AssetType:    RBTTokenType,
			PublisherDID: dc.GetDID(),
		}

		err = c.publishTxn(publishingTxn)
		if err != nil {
			c.log.Error("Failed to publish txn", "err", err)
			return &tokendetail, err
		}
	}

	return &tokendetail, nil
}

func (c *Core) FaucetTokenCheck(tokenID string, did string) model.BasicResponse {
	br := model.BasicResponse{
		Status: false,
	}
	//Cheking if token is valid
	b, err := c.getFromIPFS(tokenID)

	if err != nil {
		c.log.Error("failed to get token details from ipfs", "err", err, "token", tokenID)
		br.Message = "Cannot find token details"
		return br
	}

	tokenval := string(b)
	tokencontent := strings.Split(tokenval, ",")
	if len(tokencontent) != 3 {
		br.Message = "Non-faucet token"
		return br
	}

	faucetName := strings.TrimSpace(strings.Split(tokencontent[0], ":")[1])
	if faucetName != token.FaucetName {
		br.Message = "Invalid faucet name"
		return br
	}

	tokenLevel, err := strconv.Atoi(strings.TrimSpace(strings.Split(tokencontent[1], ":")[1]))
	if err != nil {
		br.Message = "Invalid token level"
		return br
	}

	tokenNumber, err := strconv.Atoi(strings.TrimSpace(strings.Split(tokencontent[2], ":")[1]))
	if err != nil {
		br.Message = "Invalid token number"
		return br
	}
	if tokenNumber > token.TokenMap[tokenLevel] {
		br.Message = "Invalid token number"
		return br
	}

	u, _ := url.Parse(c.faucetURL)

	u.Path = path.Join(u.Path, "/api/current-token-value")

	currentTokenValueURL := u.String()

	// Get the current value from Faucet
	resp, err := http.Get(currentTokenValueURL)
	if err != nil {
		br.Status = false
		br.Message = "Unable to fetch latest value"
		return br
	}
	defer resp.Body.Close()

	var tokendetail token.FaucetToken

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		br.Status = false
		br.Message = "Unable to fetch latest value"
		return br
	}
	//Populating the tokendetail with current token number and current token level received from Faucet.
	err = json.Unmarshal(body, &tokendetail)
	if err != nil {
		br.Status = false
		br.Message = "Unable to fetch latest value"
		return br
	}
	if tokenLevel > tokendetail.TokenLevel {
		br.Message = "Invalid token level"
		return br
	}

	//Validating token chain
	tokenType := c.TokenType(RBTString)
	genBlock := c.w.GetGenesisTokenBlock(tokenID, tokenType)

	signers, err := genBlock.GetSigner()
	if err != nil {
		br.Message = "Couldn't get signer details"
		return br
	}

	if len(signers) != 1 {
		br.Message = "Invalid signer details"
		return br
	}
	//The did will be hardcoded to match the faucet DID
	if signers[0] != "bafybmibexoa7owxdkjzfcg3ff3elqthkxsbaeznqoqq65gx6t2xkvm52fe" {
		br.Message = "Signer DID doesn't match faucet DID"
		return br
	}

	response, err := c.ValidateTokenOwner(genBlock, did)
	if err != nil {
		c.log.Error("msg", response.Message, "err", err)
		br.Message = "Token Details : " + tokenval + " Couldn't validate token chain"
		return br
	}

	br.Status = true
	br.Message = "Token owner validated successfully. Token details = " + tokenval

	return br
}

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

func (c *Core) GetMissingBlockSequence(tokenSyncInfo TokenSyncInfo) (string, string, error) {
	blockId := ""

	var blocks [][]byte
	var nextBlockID string
	var minMissingBlockId string
	var maxMissingBlockId string
	var err error

	//This for loop ensures that we fetch all the blocks in the token chain
	//starting from genesis block to latest block
	for {
		//GetAllTokenBlocks returns next 100 blocks and nextBlockID of the 100th block,
		//starting from the given block Id, in the direction: genesis to latest block
		blocks, nextBlockID, err = c.w.GetAllTokenBlocks(tokenSyncInfo.TokenID, tokenSyncInfo.TokenType, blockId)
		if err != nil {
			c.log.Error("Failed to get token chain block")
			return "", "", err
		}
		//the nextBlockID of the latest block is empty string
		blockId = nextBlockID
		if nextBlockID == "" {
			break
		}
	}

	if len(blocks) == 0 {
		c.log.Error("invalid token chain of token ", tokenSyncInfo.TokenID)
		return "", "", fmt.Errorf("missing token chain of token: %v", tokenSyncInfo.TokenID)
	}

	// calculate all the missing block numbers
	for i, blockByte := range blocks {
		if len(blocks) == i+1 {
			break
		}
		blk := block.InitBlock(blockByte, nil)
		blockHeight, err := blk.GetBlockNumber(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("failed to fetch block height")
			return "", "", err
			// TODO : handle
		}
		if blocks[i+1] == nil {
			c.log.Error("invalid block at height ", i+1)
			return "", "", fmt.Errorf("invalid block at height %v", i+1)
		}
		nextBlk := block.InitBlock(blocks[i+1], nil)
		nextBlockHeight, err := nextBlk.GetBlockNumber(tokenSyncInfo.TokenID)
		if err != nil {
			c.log.Error("failed to fetch next block height")
			return "", "", err
		}

		// if the block height difference between consecutive blocks is more than 1, that means there are a few blocks missing
		if nextBlockHeight-blockHeight > 1 {
			minMissingBlockId, err = blk.GetBlockID(tokenSyncInfo.TokenID)
			if err != nil {
				c.log.Error("failed to get min block id")
				return "", "", err
			}
			maxMissingBlockId, err = nextBlk.GetBlockID(tokenSyncInfo.TokenID)
			if err != nil {
				c.log.Error("failed to get max block id")
				return "", "", err
			}
			return minMissingBlockId, maxMissingBlockId, nil
		}
	}

	return "", "", nil
}

func (c *Core) RestartIncompleteTokenChainSyncs() {
	// read tokens to be synced from TokensTable
	tokensList, err := c.w.GetTokensToBeSynced()
	if err != nil {
		c.log.Error("failed to restart incomplete syncing, err", err)
		return
	}

	if tokensList == nil {
		return
	}
	tokenSyncMap := make(map[string][]TokenSyncInfo)
	for _, token := range tokensList {
		// fetch token type
		tokenTypeStr := RBTString
		if token.TokenValue < 1.0 {
			tokenTypeStr = PartString
		}
		tokenType := c.TokenType(tokenTypeStr)
		// fetch sender did for the respective txn id
		var senderDID string
		if token.TokenStatus == wallet.QuorumPledgedForThisToken || token.TokenStatus == wallet.TokenIsBurnt {
			senderDID = token.DID
		} else if token.TransactionID != "" {
			txnInfo, err := c.w.GetTransactionDetailsbyTransactionId(token.TransactionID)
			if err != nil {
				c.log.Error("failed to restart incomplete syncing, failed to get txn info of token ", token.TokenID)
			}
			senderDID = txnInfo.SenderDID
		}
		if c.IsDIDExist("", senderDID) {
			_ = c.w.UpdateTokenSyncStatus(token.TokenID, wallet.SyncUnrequired)
			continue
		}

		tokenSyncMap[senderDID] = append(tokenSyncMap[senderDID], TokenSyncInfo{TokenID: token.TokenID, TokenType: tokenType})
	}

	// restart all incomplete token chain sync as a background process
	go c.syncMissingBlocksOfTokenChains(tokenSyncMap)

}

// Extract token details from given genesis block and add synced tokens  to the respective tokens table of Fullnode, depending on the asset type
func (c *Core) AddTokenToRespectiveTable(tokenId string, tokenOwner string, receivedBlock ReceivedBlock, event *model.PubSubTxnInfo, syncStatus int) error {
	// var err error
	var tokenStatus int
	txnBlockType := receivedBlock.LatestBlock.GetTransType()
	if receivedBlock.LatestBlock != nil {
		switch txnBlockType {
		case block.TokenBurntType:
			tokenStatus = wallet.TokenIsBurnt
		case block.TokenGeneratedType, block.TokenTransferredType,
			block.TokenUnpledgedType, block.TokenMintedType, block.TokenMigratedType:
			tokenStatus = wallet.TokenIsFree
		case block.TokenPledgedType:
			tokenStatus = wallet.TokenIsPledged
		case block.TokenDeployedType:
			tokenStatus = wallet.TokenIsDeployed
		case block.TokenExecutedType:
			tokenStatus = wallet.TokenIsExecuted
		case block.TokenIsBurntForFT:
			tokenStatus = wallet.TokenIsBurntForFT
		case block.TokenCommittedType:
			tokenStatus = wallet.TokenIsCommitted
		case block.TokenContractCommited:
			tokenStatus = wallet.TokenIsCommitted
		case block.TokenPinnedAsService:
			tokenStatus = wallet.TokenIsPinnedAsService

		}

	}

	switch event.AssetType {
	case RBTTokenType:
		// check if token already exists in db
		syncedRBT, err := c.w.ReadSyncedRBTFromTable(tokenId)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				c.log.Debug("rbt doesn't exist, need to add new rec")
				//TODO: need to add token_status to sqlite DB of all the 4 assets
				//just before adding a token details into the sqliteDB, we will read
				tokenOwner := tokenOwner

				tokenInfo := &wallet.SyncedRBT{
					TokenID: tokenId,
					// TokenValue:    receivedBlock.GenesisBlock.GetTokenValue(),
					OwnerDID:      tokenOwner,
					BlockHash:     event.BlockHash,
					TransactionID: event.TransactionID,
					PublisherDID:  event.PublisherDID,
					BlockHeight:   event.LatestBlockHeight,
					SyncStaus:     syncStatus,
					TokenStatus:   tokenStatus,
					// TokenValue:    event.TokenValue,
				}
				// if event.TokenValue != 0 {
				// 	tokenInfo.TokenValue = event.TokenValue
				// }
				if receivedBlock.GenesisBlock != nil {
					parentTokenID, err := receivedBlock.GenesisBlock.GetParentDetials(tokenId)
					if err != nil {
						c.log.Error("failed to get the parent tokenID for the token", tokenId)
					}

					genesisBlockType := receivedBlock.GenesisBlock.GetTransType()
					if genesisBlockType == block.TokenMigratedType {
						tokenInfo.TokenValue = wholeTokenValue //No need to add parentTokenID in case of TokenMigratedType because all migrated tokens are whole tokens
					} else {
						tokenInfo.TokenValue = receivedBlock.GenesisBlock.GetTokenValue()
						tokenInfo.ParentTokenID = parentTokenID
					}

				}

				err = c.w.AddSyncedRBTToTable(tokenInfo)
				if err != nil {
					c.log.Error("failed to add synced token to fullnodeRBT table, token: ", tokenInfo.TokenID)
					return err
				}

				return nil
			} else {
				errMsg := fmt.Sprintf("error reading fullnode RBT table for token : %v", tokenId)
				c.log.Error(errMsg)
				return fmt.Errorf("%v", errMsg)
			}
		}

		c.log.Debug("rbt exists, need to update")
		// if there is no error, meaning if token exists in table, then update token info
		syncedRBT.OwnerDID = tokenOwner
		syncedRBT.TransactionID = event.TransactionID
		syncedRBT.BlockHash = event.BlockHash
		syncedRBT.SyncStaus = syncStatus
		syncedRBT.BlockHeight = event.LatestBlockHeight
		syncedRBT.PublisherDID = event.PublisherDID
		syncedRBT.TokenStatus = tokenStatus
		// if event.TokenValue != 0 {
		// 	syncedRBT.TokenValue = event.TokenValue
		// }

		//If the token value is  zero, need to update
		if syncedRBT.TokenValue == 0 {
			if receivedBlock.GenesisBlock != nil {
				genesisBlockType := receivedBlock.GenesisBlock.GetTransType()
				if genesisBlockType == block.TokenMigratedType {
					syncedRBT.TokenValue = wholeTokenValue
				} else {
					syncedRBT.TokenValue = receivedBlock.GenesisBlock.GetTokenValue()
				}

			}
		}

		err = c.w.UpdateSyncedRBTToTable(syncedRBT)
		if err != nil {
			c.log.Error("failed to update token ", tokenId)
			return err
		}

	case FTTokenType:
		// check if token already exists in db
		syncedFT, err := c.w.ReadSyncedFTFromTable(tokenId)

		if err != nil {
			if strings.Contains(err.Error(), "no records found") {

				ftInfo := &wallet.SyncedFT{
					TokenID: tokenId,
					// TokenValue:    receivedBlock.GetTokenValue(),
					CreatorDID:    event.CreatorDID,
					OwnerDID:      tokenOwner,
					PublisherDID:  event.PublisherDID,
					BlockHash:     event.BlockHash,
					BlockHeight:   event.LatestBlockHeight,
					TransactionID: event.TransactionID,
					SyncStatus:    syncStatus,
					FTName:        event.FTName,
					TokenStatus:   tokenStatus,
				}
				var comment string

				if receivedBlock.GenesisBlock != nil {
					ftInfo.TokenValue = receivedBlock.GenesisBlock.GetTokenValue()
					//If FT Name is not populated yet, get it from the genesis block comment
					if ftInfo.FTName == "" {
						comment = receivedBlock.GenesisBlock.GetComment()
						c.log.Debug("extracted comment from genesis block is :: ", comment)
						parts := strings.Split(comment, "FT Name : ")
						if len(parts) > 1 {
							ftInfo.FTName = parts[1]
						}
					}
				}

				err = c.w.AddSyncedFTToTable(ftInfo)
				if err != nil {
					c.log.Error("failed to add syncedFT token to fullnode FT table, token: ", ftInfo.TokenID)
					return err
				}
				return nil
			} else {
				errMsg := fmt.Sprintf("error reading fullnode FT table for token : %v", tokenId)
				c.log.Error(errMsg)
				return fmt.Errorf("%v", errMsg)
			}
		}

		// if there is no error, meaning if token exists in table, then update token info
		syncedFT.OwnerDID = tokenOwner
		syncedFT.PublisherDID = event.PublisherDID
		syncedFT.BlockHash = event.BlockHash
		syncedFT.BlockHeight = event.LatestBlockHeight
		syncedFT.SyncStatus = syncStatus
		syncedFT.TransactionID = event.TransactionID
		syncedFT.SyncStatus = tokenStatus

		err = c.w.UpdateSyncedFTToTable(syncedFT)
		if err != nil {
			c.log.Error("failed to update token ", tokenId)

		}
		return nil

	case SmartContractTokenType:
		// check if token already exists in db
		syncedSC, err := c.w.ReadSyncedSmartContractFromTable(tokenId)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				scDeployer := receivedBlock.GenesisBlock.GetDeployerDID()
				scInfo := &wallet.SyncedSmartContract{
					SmartContractHash: tokenId,
					Deployer:          scDeployer,
					PublisherDID:      event.PublisherDID,
					BlockHash:         event.BlockHash,
					BlockHeight:       event.LatestBlockHeight,
					TransactionID:     event.TransactionID,
					SyncStatus:        syncStatus,
					TokenStatus:       tokenStatus,
				}
				err = c.w.AddSyncedSmartContractToTable(scInfo)
				if err != nil {
					c.log.Error("failed to add smart contract to table, err ", err)
					return err
				}
				return nil
			} else {
				errMsg := fmt.Sprintf("error reading fullnode smart contract table for token : %v , err : %v", tokenId, err)
				c.log.Error(errMsg)
				return fmt.Errorf("%v", errMsg)
			}
		}

		// if there is no error, meaning if token exists in table, then update token info
		syncedSC.BlockHash = event.BlockHash
		syncedSC.BlockHeight = event.LatestBlockHeight
		syncedSC.SyncStatus = syncStatus
		syncedSC.TransactionID = event.TransactionID
		syncedSC.PublisherDID = event.PublisherDID
		syncedSC.TokenStatus = syncStatus

		err = c.w.UpdateSyncedSmartContractToTable(syncedSC)
		if err != nil {
			c.log.Error("failed to update token ", tokenId)
			return err
		}
	case NFTTokenType:
		// check if token already exists in db
		syncedNFT, err := c.w.ReadSyncedNFTFromTable(tokenId)
		if err != nil {

			if strings.Contains(err.Error(), "no records found") {
				c.log.Debug("nft doesn't exist, creating new record")

				nftOwner := receivedBlock.LatestBlock.GetDeployerDID()
				nftInfo := &wallet.SyncedNFT{
					TokenID:       tokenId,
					OwnerDID:      nftOwner,
					PublisherDID:  event.PublisherDID,
					BlockHash:     event.BlockHash,
					BlockHeight:   event.LatestBlockHeight,
					TransactionID: event.TransactionID,
					SyncStatus:    syncStatus,
					TokenStatus:   tokenStatus,
				} // TODO : add metadata details
				if receivedBlock.GenesisBlock != nil {
					nftInfo.TokenValue = receivedBlock.GenesisBlock.GetTokenValue()
				}
				err = c.w.AddSyncedNFTToTable(nftInfo)

				if err != nil {
					c.log.Error("failed to add synced NFT Token to fullnode NFT Table, token: ", nftInfo.TokenID)
					return err
				}

				return nil
			} else {
				errMsg := fmt.Sprintf("error reading fullnode NFT table for token : %v", tokenId)
				c.log.Error(errMsg)
				return fmt.Errorf("%v", errMsg)
			}
		}
		c.log.Debug("nft exists, updating info")
		// if there is no error, meaning if token exists in table, then update token info
		syncedNFT.BlockHash = event.BlockHash
		syncedNFT.BlockHeight = event.LatestBlockHeight
		syncedNFT.OwnerDID = tokenOwner
		syncedNFT.PublisherDID = event.PublisherDID
		syncedNFT.SyncStatus = syncStatus
		syncedNFT.TransactionID = event.TransactionID
		syncedNFT.TokenStatus = tokenStatus

		err = c.w.UpdateSyncedNFTToTable(syncedNFT)
		if err != nil {
			c.log.Error("failed to update token ", tokenId)
			return err
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
		tokenHash, _ := c.ipfs.Add(
			bytes.NewBufferString(tokenId),
		)

		tokenContent, err = c.w.Cat(tokenHash, wallet.FullNodeRole, c.peerID)
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
		var nft NFTIpfsInfo
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
		var smartContractIpfsInfo SmartContractToken
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

func (c *Core) StoreSmartContractFilesToPSQL(smartContractHash string, smartContractIpfsContent SmartContractToken) error {
	// Fetch the binary code file
	binaryCodeFile, err := c.ipfsOps.Cat(smartContractIpfsContent.BinaryCodeHash)
	if err != nil {
		c.log.Error("Failed to fetch binary code file from network", "err", err)
		return err
	}
	defer binaryCodeFile.Close()

	binaryCodeFileName := "binaryCodeFile.wasm"

	// Read the content of binaryCodeFile
	binaryCodeContent, err := io.ReadAll(binaryCodeFile)
	if err != nil {
		c.log.Error("Failed to read binary code file", "err", err)
		return err
	}

	// Fetch and store the raw code file
	rawCodeFile, err := c.ipfsOps.Cat(smartContractIpfsContent.RawCodeHash)
	if err != nil {
		c.log.Error("Failed to fetch raw code file from IPFS", "err", err)
		return err
	}
	defer rawCodeFile.Close()

	rawCodeFileName := "rawCodeFile"

	// Read the content of rawCodeFile
	rawCodeContent, err := io.ReadAll(rawCodeFile)
	if err != nil {
		c.log.Error("Failed to read raw code file", "err", err)
		return err
	}

	// Add smart contract IPFS content in PSQL db
	smartContractContent := &wallet.SmartContractContent{
		SmartContractHash:  smartContractHash,
		DeployerDID:        smartContractIpfsContent.DID,
		BinaryCodeFileName: binaryCodeFileName,
		BinaryCode:         binaryCodeContent,
		RawCodeFileName:    rawCodeFileName,
		RawCode:            rawCodeContent,
	}
	err = c.w.AddSmartContractContentToPSQl(smartContractContent)
	if err != nil {
		errMsg := fmt.Sprintf("failed to add smart contract content to psql, smart contract hash : %v, error: %v", smartContractHash, err)
		c.log.Error(errMsg)
		return fmt.Errorf(errMsg)
	}

	c.log.Info("Successfully stored all smart contract files")
	return nil
}

func (c *Core) ReadTokenFromFullnodeTokensTable(assetType int, tokenId string) (uint64, string, string, error) {
	switch assetType {
	case RBTTokenType:
		rbt, err := c.w.ReadSyncedRBTFromTable(tokenId)
		if err != nil {
			return 0, "", "", err
		}
		return rbt.BlockHeight, rbt.BlockHash, rbt.OwnerDID, nil
	case FTTokenType:
		ft, err := c.w.ReadSyncedFTFromTable(tokenId)
		if err != nil {
			return 0, "", "", err
		}
		return ft.BlockHeight, ft.BlockHash, ft.OwnerDID, nil
	case NFTTokenType:
		nft, err := c.w.ReadSyncedNFTFromTable(tokenId)
		if err != nil {
			return 0, "", "", err
		}
		return nft.BlockHeight, nft.BlockHash, nft.OwnerDID, nil
	case SmartContractTokenType:
		sc, err := c.w.ReadSyncedSmartContractFromTable(tokenId)
		if err != nil {
			return 0, "", "", err
		}
		return sc.BlockHeight, sc.BlockHash, sc.Deployer, nil
	default:
		c.log.Error("invalid asset type")
		return 0, "", "", fmt.Errorf("invalid asset type")
	}
}

// IsDoubleSpendOrValidationError returns true if err indicates a double-spend or block validation
// failure (chain mismatch, owner mismatch, or signature error).
func IsDoubleSpendOrValidationError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "previous blockID of the blk which is getting added is not matching with the blockID which is present") ||
		strings.Contains(s, "Owner of the latest blockID is not matchig with") ||
		strings.Contains(s, "signature validation error") ||
		strings.Contains(s, "invalid block signature for token")
}

// HandleSyncErrorAsDoubleSpent stores double-spent token info and logs when syncOrValidationErr
// is a double-spend/validation error. Returns (true, nil) when handled and stored, (true, storeErr)
// when storage failed, and (false, syncOrValidationErr) when the error is not a double-spend type.
func (c *Core) HandleSyncErrorAsDoubleSpent(syncOrValidationErr error, tokenId string, assetType, tokenType int, publisherDID, claimedOwnerI, claimedOwnerII, errMessage string) (handled bool, retErr error) {
	if !IsDoubleSpendOrValidationError(syncOrValidationErr) {
		return false, syncOrValidationErr
	}
	info := &model.DoubleSpentTokenInfo{
		TokenID:        tokenId,
		AssetType:      assetType,
		TokenType:      tokenType,
		PublisherDID:   publisherDID,
		ClaimedOwnerI:  claimedOwnerI,
		ClaimedOwnerII: claimedOwnerII,
		ErrorMessage:   errMessage,
	}
	storeErr := c.StoreDoubleSpentTokenInfo(info)
	if storeErr != nil {
		c.log.Error("failed to update double spent token", "token", tokenId, "err", storeErr)
		return true, storeErr
	}
	c.log.Error("token has been added to double spent token's table", "token", tokenId, "error", syncOrValidationErr)
	return true, nil
}

// Store double spent tokens in fullnode DB for later analysis
func (c *Core) StoreDoubleSpentTokenInfo(doubleSpentTokenInfo *model.DoubleSpentTokenInfo) error {
	err := c.w.AddDoubleSpentTokenInfo(doubleSpentTokenInfo)
	if err != nil {
		return err
	}

	switch doubleSpentTokenInfo.AssetType {
	case RBTTokenType:
		err = c.w.RemoveSyncedRBTFromTable(doubleSpentTokenInfo.TokenID)
	case FTTokenType:
		err = c.w.RemoveSyncedFTFromTable(doubleSpentTokenInfo.TokenID)
	case NFTTokenType:
		err = c.w.RemoveSyncedNFTFromTable(doubleSpentTokenInfo.TokenID)
	case SmartContractTokenType:
		err = c.w.RemoveSyncedSmartContractFromTable(doubleSpentTokenInfo.TokenID)
	default:
		err = fmt.Errorf("invalid asset type, failed to remove double spent token from table, token : %v", doubleSpentTokenInfo.TokenID)
	}
	return err
}

func (c *Core) relaseToken(release *bool, token string) {
	if *release {
		c.w.ReleaseToken(token)
	}
}

func (c *Core) ValidateNewTokenContent(tokenContent string) error {
	devidedParts := strings.Split(tokenContent, "_")

	tokenTypeString := RBTString
	if len(devidedParts) == 3 {
		tokenTypeString = PartString
	}
	tokenType := c.TokenType(tokenTypeString)

	// parse level (e.g. "002")
	level, err := strconv.Atoi(strings.TrimLeft(devidedParts[0], "0"))
	if err != nil {
		return fmt.Errorf("invalid token level in token content: %s", tokenContent)
	}

	// parse token number (e.g. "1000")
	tokenNo, err := strconv.Atoi(devidedParts[1])
	if err != nil {
		return fmt.Errorf("invalid token number in token content: %s", tokenContent)
	}

	switch tokenType {
	case token.TestTokenType, token.TestPartTokenType:
		// Testnet: level is 10000+ (10000 = TokenMap level 0, 10001 = level 1, ...)
		if level < token.LocalTestTokenLevelBase {
			return fmt.Errorf(
				"invalid testnet token level %d: testnet level must be >= %d",
				level, token.LocalTestTokenLevelBase,
			)
		}
		mapLevel := level - token.LocalTestTokenLevelBase
		maxAllowed, ok := token.TokenMap[mapLevel]
		if !ok {
			return fmt.Errorf(
				"invalid testnet token level %d: (level-%d=%d) not present in TokenMap",
				level, token.LocalTestTokenLevelBase, mapLevel,
			)
		}
		if tokenNo < 0 || tokenNo > maxAllowed {
			return fmt.Errorf(
				"testnet token number %d exceeds max allowed %d for level %d",
				tokenNo, maxAllowed, level,
			)
		}
		c.log.Debug("token content validated for the testtoken", tokenContent)
	case token.RBTTokenType, token.PartTokenType:
		// Mainnet: level is used directly as TokenMap key (0, 1, ... 78)
		maxAllowed, ok := token.TokenMap[level]
		if !ok {
			return fmt.Errorf(
				"invalid mainnet token level %d: not present in TokenMap",
				level,
			)
		}
		// if level exists in TokenMap, validate token number
		if tokenNo < 0 || tokenNo > maxAllowed {
			return fmt.Errorf(
				"mainnet token number %d exceeds max allowed %d for level %d",
				tokenNo, maxAllowed, level,
			)
		}
		c.log.Debug("token content validated for the MainNet token", tokenContent)
	}

	MaxPossiblePartTokenNumber := parts.MaxPossiblePartsIndexByMaxDecimalPlaces(uint(MaxDecimalPlaces))
	if tokenTypeString == PartString {
		partTokenNumber, err := strconv.Atoi(devidedParts[2])
		if err != nil {
			return fmt.Errorf("invalid part number in token content: %s", tokenContent)
		}
		if partTokenNumber > MaxPossiblePartTokenNumber {
			return fmt.Errorf(
				"Parttoken number %d exceeds max allowed %d ",
				partTokenNumber, MaxPossiblePartTokenNumber,
			)

		}
		c.log.Debug("token content validated for the part token", tokenContent)

	}

	return nil
}

func (c *Core) GetTokenContentAndValidate(tokenId string, assetType int) error {
	// ✅ Validation applies ONLY to RBT tokens
	if assetType != RBTTokenType {
		return nil
	}

	if tokenId != "" {
		// Validate RBT token content against TokenMap
		if err := c.ValidateNewTokenContent(tokenId); err != nil {
			c.log.Error(
				"Invalid RBT token content",
				"tokenId", tokenId,
				"err", err,
			)
			return err
		}

	}

	c.log.Debug("Token content validated successfully for the token", tokenId)

	return nil
}
