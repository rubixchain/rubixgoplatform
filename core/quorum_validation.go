package core

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/rac"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

type TokenStateCheckResult struct {
	Token                 string
	Exhausted             bool
	Error                 error
	Message               string
	tokenIDTokenStateData string
	tokenIDTokenStateHash string
}

const (
	maxRetries     = 3
	baseRetryDelay = 500 * time.Millisecond
)

func retry(operation func() error) error {
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil
		}
		sleep := backoff(attempt)
		time.Sleep(sleep)
	}
	return err
}

func backoff(attempt int) time.Duration {
	jitter := time.Duration(rand.Intn(100)) * time.Millisecond
	return time.Duration(math.Pow(2, float64(attempt-1)))*baseRetryDelay + jitter
}

func recordFirstError(errPtr *error, err error, once *sync.Once) {
	once.Do(func() {
		*errPtr = err
	})
}

func (c *Core) validateSigner(b *block.Block, selfDID string, p *ipfsport.Peer) (bool, error) {
	signers, err := b.GetSigner()
	if err != nil {
		c.log.Error("failed to get signers", "err", err)
		return false, fmt.Errorf("failed to get signers", "err", err)
	}
	c.log.Debug("Signers", signers)
	for _, signer := range signers {
		var dc did.DIDCrypto
		switch b.GetTransType() {
		case block.TokenGeneratedType, block.TokenBurntType:
			dc, err = c.SetupForienDID(signer, selfDID)
			if err != nil {
				c.log.Error("failed to setup foreign DID", "err", err)
				return false, fmt.Errorf("failed to setup foreign DID : ", signer, "err", err)
			}
		default:
			signerInfo, err := c.GetPeerDIDInfo(signer)
			if err != nil {
				if strings.Contains(err.Error(), "retry") {
					c.AddPeerDetails(*signerInfo)
				}
			}
			if signerInfo == nil || *signerInfo.DIDType == -1 {
				peerDetails, err := c.GetPeerInfo(p, signer)
				if err != nil || peerDetails.PeerInfo.DIDType == nil {
					c.log.Debug("quorum does not have did type of prev-block signer ", signer)
					peerUpdateResult, err := c.w.UpdatePeerDIDType(signer, did.BasicDIDMode)
					if !peerUpdateResult || err != nil {
						*signerInfo.DIDType = did.BasicDIDMode
						c.AddPeerDetails(*signerInfo)
					}
				} else {
					peerUpdateResult, err := c.w.UpdatePeerDIDType(signer, *peerDetails.PeerInfo.DIDType)
					if !peerUpdateResult || err != nil {
						*signerInfo.DIDType = did.BasicDIDMode
						c.AddPeerDetails(*signerInfo)
					}
				}
			}
			dc, err = c.SetupForienDIDQuorum(signer, selfDID)
			if err != nil {
				c.log.Error("failed to setup foreign DID quorum", "err", err)
				return false, fmt.Errorf("failed to setup foreign DID quorum : %v, err : %v ", signer, err)
			}
		}
		err := b.VerifySignature(dc)
		if err != nil {
			if dc.GetSignType() == did.NlssVersion {
				peerUpdateResult, err := c.w.UpdatePeerDIDType(signer, did.LiteDIDMode)
				if !peerUpdateResult || err != nil {
					liteDID := did.LiteDIDMode
					signerInfo := wallet.DIDPeerMap{
						DID:     signer,
						DIDType: &liteDID,
					}
					c.AddPeerDetails(signerInfo)
				}
				dc, err = c.SetupForienDIDQuorum(signer, selfDID)
				if err != nil {
					c.log.Error("failed to setup foreign DID quorum", "err", err)
					return false, fmt.Errorf("failed to setup foreign DID quorum : %v err: %v", signer, err)
				}
				err = b.VerifySignature(dc)
				if err != nil {
					c.log.Error("Failed to verify signature", "err", err)
					return false, fmt.Errorf("failed to verify signature, err: %v", err)
				}
			} else {
				c.log.Error("Failed to verify signature", "err", err)
				return false, fmt.Errorf("failed to verify signature, err: %v", err)
			}
		}
	}
	return true, nil
}

func (c *Core) syncParentToken(p *ipfsport.Peer, pt string) (int, error) {
	var issueType int
	b, err := c.getFromIPFS(pt)
	if err != nil {
		c.log.Error("failed to get parent token details from ipfs", "err", err, "token", pt)
		return -1, err
	}
	_, iswholeToken, _ := token.CheckWholeToken(string(b), c.testNet)

	tt := token.RBTTokenType
	tv := float64(1)
	if !iswholeToken {
		blk := util.StrToHex(string(b))
		rb, err := rac.InitRacBlock(blk, nil)
		if err != nil {
			c.log.Error("invalid token, invalid rac block", "err", err)
			return -1, err
		}
		tt = rac.RacType2TokenType(rb.GetRacType())
		if c.TokenType(PartString) == tt {
			tv = rb.GetRacValue()
		}
	}
	lbID := ""
	// lb := c.w.GetLatestTokenBlock(pt, tt)
	// if lb != nil {
	// 	lbID, err = lb.GetBlockID(pt)
	// 	if err != nil {
	// 		lbID = ""
	// 	}
	// }
	err, syncResponse := c.syncTokenChainFrom(p, lbID, pt, tt)
	if err != nil {
		c.log.Error("failed to sync token chain block", "err", err, "syncResponse", syncResponse)
		return -1, fmt.Errorf(" failed to sync tokenchain Parent Token: %v, issueType: %v", pt, TokenChainNotSynced)
	}
	ptb := c.w.GetLatestTokenBlock(pt, tt)
	if ptb == nil {
		c.log.Error("Failed to get latest token chain block", "token", pt)
		return -1, fmt.Errorf("failed to get latest block")
	}
	td, err := c.w.ReadToken(pt)
	if err != nil {
		td = &wallet.Token{
			TokenID:     pt,
			TokenValue:  tv,
			DID:         p.GetPeerDID(),
			TokenStatus: wallet.TokenIsBurnt,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if c.TokenType(PartString) == tt {
			gb := c.w.GetGenesisTokenBlock(pt, tt)
			if gb == nil {
				c.log.Error("failed to get genesis token chain block", "token", pt)
				return -1, fmt.Errorf("failed to get genesis token chain block")
			}
			ppt, _, err := gb.GetParentDetials(pt)
			if err != nil {
				c.log.Error("failed to get genesis token chain block", "token", pt, "err", err)
				return -1, fmt.Errorf("failed to get genesis token chain block")
			}
			td.ParentTokenID = ppt
		}
		c.w.CreateToken(td)
	} else {
		td.TokenStatus = wallet.TokenIsBurnt
		c.w.UpdateToken(td)
	}
	// update sync status to incomplete
	if td.SyncStatus == wallet.SyncUnrequired {
		err = c.w.UpdateTokenSyncStatus(pt, wallet.SyncIncomplete)
		if err != nil {
			if !strings.Contains(err.Error(), "no records found") {
				c.log.Error("failed to update parent token sync status as incomplete, token ", pt)
			}
		}

	}
	if ptb.GetTransType() != block.TokenBurntType {
		issueType = ParentTokenNotBurned // parent token is not in burnt stage
		//Commenting gps
		//fmt.Println("block state is ", ptb.GetTransTokens(), " expected value is ", block.TokenBurntType)
		c.log.Error("parent token is not in burnt stage", "token", pt)
		return -1, fmt.Errorf("parent token is not in burnt stage. pt: %v, issueType: %v", pt, issueType)
	}
	return tt, nil
}
func (c *Core) validateSingleToken(cr *ConensusRequest, sc *contract.Contract, quorumDID string, ti contract.TokenInfo, p *ipfsport.Peer, address, receiverAddress string) (error, bool) {
	// Skip DHT check in trusted network mode
	if !c.cfg.CfgData.TrustedNetwork {
		if ids, err := c.GetDHTddrs(ti.Token); err != nil || len(ids) == 0 {
			c.log.Debug("Skipping token", "token", ti.Token, "reason", "no DHT entries found")
			return nil, false // skip token if no DHT entries found
		}
	}

	err, syncResponse := c.syncTokenChainFrom(p, ti.BlockID, ti.Token, ti.TokenType)
	if err != nil {
		if strings.Contains(err.Error(), "syncer block height discrepency") {
			c.log.Debug("Token has sync issue", "token", ti.Token, "err", err, "syncResponse", syncResponse)
			// logic for handling block height discrepancy if needed
			return nil, true // mark this token as having a sync issue
		}
		c.log.Error("Failed to sync token chain for token", "token", ti.Token, "err", err, "syncResponse", syncResponse)
		return err, true
	}

	genesisBlock := c.w.GetGenesisTokenBlock(ti.Token, ti.TokenType)
	if genesisBlock == nil {
		c.log.Error("Failed to get first token chain block for token", "token", ti.Token)
		return fmt.Errorf("failed to get first token chain block %v", ti.Token), false
	}

	if c.TokenType(PartString) == ti.TokenType {
		parentToken, _, err := genesisBlock.GetParentDetials(ti.Token)
		if err != nil {
			c.log.Error("Failed to get parent token for token", "token", ti.Token, "err", err)
			return err, false
		}
		parentTokenType, err := c.syncParentToken(p, parentToken)
		if err != nil || parentTokenType == -1 {
			c.log.Error("Failed to sync parent token for token", "token", ti.Token, "err", err)
			return err, false
		}
		_, err = c.w.Pin(parentToken, wallet.ParentTokenPinByQuorumRole, quorumDID, cr.TransactionID, address, receiverAddress, ti.TokenValue)
		if err != nil {
			c.log.Error("Failed to pin parent token for token", "token", ti.Token, "err", err)
			return err, false
		}
	}

	if ti.TokenType == token.RBTTokenType {
		tl, tn, err := genesisBlock.GetTokenDetials(ti.Token)
		if err != nil {
			c.log.Error("Failed to get token details for token", "token", ti.Token, "err", err)
			return err, false
		}
		tid, err := IpfsAddWithBackoff(c.ipfs, bytes.NewBufferString(token.GetTokenString(tl, tn)), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if err != nil {
			c.log.Error("Failed to pin token hash for token", "token", ti.Token, "err", err)
			return err, false
		}
		if tid != ti.Token {
			c.log.Error("Invalid token hash for token", "token", ti.Token, "expected", tid, "actual", ti.Token)
			return fmt.Errorf("Invalid token hash for %s", ti.Token), false
		}
	}

	b := c.w.GetLatestTokenBlock(ti.Token, ti.TokenType)
	if b == nil {
		c.log.Info("DEBUG BLOCK LIST LOG REACHED (quorum)", "token", ti.Token)
		// Debug log: print all block IDs for this token
		blocks, _, _ := c.w.GetAllTokenBlocks(ti.Token, ti.TokenType, "")
		blockIDs := make([]string, 0, len(blocks))
		for _, blkBytes := range blocks {
			blk := block.InitBlock(blkBytes, nil)
			if blk != nil {
				bid, err := blk.GetBlockID(ti.Token)
				if err == nil {
					blockIDs = append(blockIDs, bid)
				}
			}
		}
		c.log.Debug("Token chain block list for token", "token", ti.Token, "blockIDs", blockIDs)
		return fmt.Errorf("Invalid token chain block for %s", ti.Token), false
	}
	c.log.Info("DEBUG LATEST BLOCK LOG REACHED (quorum)", "token", ti.Token)
	// Debug log: print latest block details
	blockID, _ := b.GetBlockID(ti.Token)
	blockHash, _ := b.GetHash()
	c.log.Debug("Latest block for token", "token", ti.Token, "blockID", blockID, "blockHash", blockHash, "owner", b.GetOwner())

	if b.GetPinningNodeDID() != "" && b.GetOwner() != sc.GetSenderDID() {
		c.log.Error("Invalid token owner for token", "token", ti.Token, "expected", sc.GetSenderDID(), "actual", b.GetOwner())
		return fmt.Errorf("invalid token owner: token pinned as service, token %s", ti.Token), false
	}

	valid, err := c.validateSigner(b, quorumDID, p)
	if !valid || err != nil {
		c.log.Error("Failed to validate token signer for token", "token", ti.Token, "err", err)
		return fmt.Errorf("failed to validate token signer for %s", ti.Token), false
	}

	tokenInfo, err := c.w.ReadToken(ti.Token)
	if err != nil || tokenInfo.TokenID == "" {
		tokenInfo = &wallet.Token{
			TokenID:       ti.Token,
			TokenValue:    ti.TokenValue,
			ParentTokenID: "",
			DID:           ti.OwnerDID,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		dbWriteSem <- struct{}{}
		err := util.RetrySQLiteWrite(func() error {
			return c.w.CreateToken(tokenInfo)
		}, 10, 100*time.Millisecond)
		<-dbWriteSem
		if err != nil {
			c.log.Error("Failed to create token for token", "token", ti.Token, "err", err)
			return err, false
		}
	}

	tokenInfo.DID = sc.GetSenderDID()
	tokenInfo.TokenStatus = wallet.QuorumPledgedForThisToken
	tokenInfo.TransactionID = b.GetTid()
	tokenInfo.SyncStatus = wallet.SyncIncomplete
	dbWriteSem <- struct{}{}
	err = util.RetrySQLiteWrite(func() error {
		return c.w.UpdateToken(tokenInfo)
	}, 10, 100*time.Millisecond)
	<-dbWriteSem
	if err != nil {
		c.log.Error("Failed to update token for token", "token", ti.Token, "err", err)
		return err, false
	}

	return nil, false
}

func (c *Core) validateTokenOwnership(cr *ConensusRequest, sc *contract.Contract, quorumDID string) (bool, error, []string) {
	var ti []contract.TokenInfo
	var address, receiverAddress string

	if cr.Mode == SmartContractDeployMode || cr.Mode == NFTDeployMode {
		ti = sc.GetCommitedTokensInfo()
		address = cr.DeployerPeerID + "." + sc.GetDeployerDID()
	} else {
		ti = sc.GetTransTokenInfo()
		address = cr.SenderPeerID + "." + sc.GetSenderDID()
		receiverAddress = cr.ReceiverPeerID + "." + sc.GetReceiverDID()
	}

	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return false, err, nil
	}
	defer p.Close()

	type tokenValidationResult struct {
		Token     string
		Err       error
		SyncIssue bool
	}

	numCores := runtime.NumCPU()
	maxWorkers := numCores * 2
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	results := make(chan tokenValidationResult, len(ti))

	for _, tokenInfo := range ti {
		wg.Add(1)
		sem <- struct{}{}
		go func(t contract.TokenInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			// 1. Sync token chain before validation
			err, syncResp := c.syncTokenChainFrom(p, t.BlockID, t.Token, t.TokenType)
			if err != nil {
				c.log.Debug("syncResponse", syncResp)
				if strings.Contains(err.Error(), "syncer block height discrepency") {
					c.log.Info("Block height discrepancy detected during sync", "token", t.Token, "err", err)
					parts := strings.SplitN(err.Error(), "|", 2)
					blockID := ""
					if len(parts) > 1 {
						blockID = parts[1]
					}
					// Fetch local latest block
					localBlk := c.w.GetLatestTokenBlock(t.Token, t.TokenType)
					if localBlk != nil {
						localBlockID, _ := localBlk.GetBlockID(t.Token)
						localBlockHash, _ := localBlk.GetHash()
						c.log.Debug("Local latest block", "token", t.Token, "blockID", localBlockID, "blockHash", localBlockHash, "owner", localBlk.GetOwner())
					}
					// Fetch remote block if possible (from all token blocks)
					blocks, _, _ := c.w.GetAllTokenBlocks(t.Token, t.TokenType, "")
					if len(blocks) > 0 && blockID != "" {
						for _, blkBytes := range blocks {
							blk := block.InitBlock(blkBytes, nil)
							if blk != nil {
								bid, _ := blk.GetBlockID(t.Token)
								if bid == blockID {
									blockHash, _ := blk.GetHash()
									c.log.Debug("Remote block (from all blocks)", "token", t.Token, "blockID", bid, "blockHash", blockHash, "owner", blk.GetOwner())
								}
							}
						}
					}
					// Remove latest block and retry sync
					errRemove := c.w.RemoveTokenChainBlocklatest(t.Token, t.TokenType)
					if errRemove != nil {
						c.log.Error("Failed to remove latest block during discrepancy resolution", "token", t.Token, "err", errRemove)
					}
					c.log.Info("Retrying syncTokenChainFrom after removing latest block", "token", t.Token)
					errRetry, syncResp := c.syncTokenChainFrom(p, t.BlockID, t.Token, t.TokenType)
					if errRetry != nil {
						c.log.Error(
							"Failed to sync token chain in token validation (retry)",
							"token", t.Token,
							"blockID", t.BlockID,
							"tokenType", t.TokenType,
							"peerID", p.GetPeerID(),
							"peerDID", p.GetPeerDID(),
							"syncResponse", syncResp,
							"err", errRetry,
						)
						c.log.Error("Retry syncTokenChainFrom failed after discrepancy resolution", "token", t.Token, "err", errRetry)
						// Commenting out marking as sync issue for further investigation
						// results <- tokenValidationResult{Token: t.Token, Err: errRetry, SyncIssue: true}
						results <- tokenValidationResult{Token: t.Token, Err: errRetry, SyncIssue: false}
						return
					}
				} else {
					c.log.Error("Failed to sync token chain block", "token", t.Token, "err", err)
					results <- tokenValidationResult{Token: t.Token, Err: err, SyncIssue: true}
					return
				}
			}

			// 2. Validate token as before
			err, syncIssue := c.validateSingleToken(cr, sc, quorumDID, t, p, address, receiverAddress)
			results <- tokenValidationResult{Token: t.Token, Err: err, SyncIssue: syncIssue}
		}(tokenInfo)
	}

	wg.Wait()
	close(results)

	var syncIssueTokens []string
	for res := range results {
		if res.Err != nil {
			c.log.Error("Token validation failed", "token", res.Token, "err", res.Err)
			if res.SyncIssue {
				syncIssueTokens = append(syncIssueTokens, res.Token)
			} else {
				return false, res.Err, nil
			}
		}
	}

	if len(syncIssueTokens) > 0 {
		return false, fmt.Errorf("failed to sync tokenchain Token: issueType: %v", TokenChainNotSynced), syncIssueTokens
	}

	return true, nil, nil
}

// BatchSyncTokenInfo holds information for batch token syncing
type BatchSyncTokenInfo struct {
	Token     string
	BlockID   string
	TokenType int
}

// BlockValidationResult represents the result of validating a specific block
// Used for optimized token validation
type BlockValidationResult struct {
	BlockHash string
	Block     *block.Block
	Tokens    []string // List of tokens that share this block
	IsValid   bool
	Error     error
	SyncIssue bool
}

// validateTokenOwnershipOptimized groups tokens by their latest block and validates each unique block only once
func (c *Core) validateTokenOwnershipOptimized(cr *ConensusRequest, sc *contract.Contract, quorumDID string) (bool, error, []string) {
	// Track overall validation time
	defer c.TrackOperation("quorum.validate_token_ownership.total", map[string]interface{}{
		"transaction_id": cr.TransactionID,
		"mode": cr.Mode,
		"token_count": len(sc.GetTransTokenInfo()),
	})(nil)
	
	var ti []contract.TokenInfo
	var address string

	if cr.Mode == SmartContractDeployMode || cr.Mode == NFTDeployMode {
		ti = sc.GetCommitedTokensInfo()
		address = cr.DeployerPeerID + "." + sc.GetDeployerDID()
	} else {
		ti = sc.GetTransTokenInfo()
		address = cr.SenderPeerID + "." + sc.GetSenderDID()
	}

	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return false, err, nil
	}
	defer p.Close()

	// Pre-optimization: Use genesis batch optimization for RBT/Part tokens if applicable
	var tokenCreatorCache map[string]string
	if cr.Mode == RBTTransferMode || cr.Mode == SelfTransferMode {
		// Check if we have many part tokens that would benefit from batch genesis optimization
		partTokenCount := 0
		for _, token := range ti {
			if token.TokenType == c.TokenType(PartString) {
				partTokenCount++
			}
		}
		
		// Use batch genesis optimization if we have significant part tokens
		if partTokenCount > 10 {
			c.log.Info("Using batch genesis optimization for RBT validation",
				"total_tokens", len(ti),
				"part_tokens", partTokenCount)
			
			genesisBatchOptimizer := wallet.NewGenesisBatchOptimizer(c.w)
			tokenCreatorCache = genesisBatchOptimizer.BatchProcessTokens(ti)
			
			c.log.Debug("Genesis batch optimization complete",
				"cache_size", len(tokenCreatorCache))
		}
	}

	// Step 1: Collect tokens that need syncing and group tokens by their latest block hash
	// Track collect phase
	_ = time.Now() // collectStart - reserved for future phase timing
	blockGroups := make(map[string]*BlockValidationResult)
	var tokensNeedingSync []BatchSyncTokenInfo
	syncNeeded := 0
	syncSkipped := 0

	c.log.Info("Starting optimized token validation", "totalTokens", len(ti))
	c.log.Debug("Analyzing tokens for sync requirements", "totalTokens", len(ti))

	for _, tokenInfo := range ti {
		var signersForExistingBlock []string

		// Check if the current quorum was also part of the previous transaction
		latestBlock := c.w.GetLatestTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
		if latestBlock == nil {
			// Try genesis block as fallback
			latestBlock = c.w.GetGenesisTokenBlock(tokenInfo.Token, tokenInfo.TokenType)
			if latestBlock == nil {
				c.log.Debug("No latest or genesis block found - token needs sync", "token", tokenInfo.Token)
			} else {
				c.log.Debug("Using genesis block for token sync check", "token", tokenInfo.Token)
				signersForExistingBlock, err = latestBlock.GetSigner()
				if err != nil {
					return false, fmt.Errorf("failed to extract Quorums from genesis block: %v", err), nil
				}
			}
		} else {
			signersForExistingBlock, err = latestBlock.GetSigner()
			if err != nil {
				return false, fmt.Errorf("failed to extract Quorums from the Contract struct input"), nil
			}
		}

		if !isPresentInList(signersForExistingBlock, quorumDID) || len(signersForExistingBlock) == 0 {
			// This token needs syncing - collect it for batch processing
			// Don't log for each token to reduce noise
			// Validate token info before adding to sync list
			if tokenInfo.Token == "" {
				c.log.Error("Empty token in input list",
					"index", len(tokensNeedingSync),
					"tokenType", tokenInfo.TokenType,
					"blockID", tokenInfo.BlockID)
				return false, fmt.Errorf("empty token found in input at position %d", len(tokensNeedingSync)), nil
			}
			
			// Create BatchSyncTokenInfo - don't use pool here as we need to store them
			tokensNeedingSync = append(tokensNeedingSync, BatchSyncTokenInfo{
				Token:     tokenInfo.Token,
				BlockID:   tokenInfo.BlockID,
				TokenType: tokenInfo.TokenType,
			})
			syncNeeded++
		} else {
			// Quorum already signed - no sync needed
			// Don't log for each token to reduce noise
			syncSkipped++
			
			//Store the latest block we already have
			blockHash, err := latestBlock.GetHash()
			if err != nil {
				c.log.Error("Failed to get block hash for token", "token", tokenInfo.Token, "err", err)
				return false, fmt.Errorf("failed to get block hash for token %s: %v", tokenInfo.Token, err), nil
			}

			// Group tokens by block hash
			if result, exists := blockGroups[blockHash]; exists {
				result.Tokens = append(result.Tokens, tokenInfo.Token)
			} else {
				blockGroups[blockHash] = &BlockValidationResult{
					BlockHash: blockHash,
					Block:     latestBlock,
					Tokens:    []string{tokenInfo.Token},
				}
			}
		}
	}

	c.log.Info("Sync analysis completed", 
		"totalTokens", len(ti),
		"needSync", syncNeeded,
		"skipSync", syncSkipped,
		"uniqueBlocks", len(blockGroups))
	
	// Track collection phase time
	c.TrackOperation("quorum.validate_token_ownership.collect_tokens", map[string]interface{}{
		"total_tokens": len(ti),
		"need_sync": syncNeeded,
		"skip_sync": syncSkipped,
	})(nil)

	// Step 2: Sync tokens that need it
	if len(tokensNeedingSync) > 0 {
		syncStartTime := time.Now()
		
		if len(tokensNeedingSync) > 10 {
			// Use batch sync for large token sets
			c.log.Info("Using batch sync for tokens", "count", len(tokensNeedingSync))
			_ = time.Now() // batchSyncStart - reserved for future phase timing
			err := c.syncTokensInBatch(p, tokensNeedingSync)
			c.TrackOperation("quorum.validate_token_ownership.batch_sync", map[string]interface{}{
				"token_count": len(tokensNeedingSync),
			})(err)
			if err != nil {
				return false, err, nil
			}
		} else {
			// Use parallel sync for small token sets (≤10 tokens)
			c.log.Info("Using parallel sync for tokens", "count", len(tokensNeedingSync))
			
			// Limit concurrency to 10 (matching IPFS limits)
			sem := make(chan struct{}, 10)
			var syncWg sync.WaitGroup
			var syncErr error
			var syncMu sync.Mutex
			
			for _, syncInfo := range tokensNeedingSync {
				syncWg.Add(1)
				go func(si BatchSyncTokenInfo) {
					defer syncWg.Done()
					
					sem <- struct{}{}
					defer func() { <-sem }()
					
					c.log.Debug("Syncing token chain before validation", "token", si.Token)
					err, syncResp := c.syncTokenChainFrom(p, si.BlockID, si.Token, si.TokenType)
					if err != nil {
						c.log.Error(
							"Failed to sync token chain in token validation",
							"token", si.Token,
							"blockID", si.BlockID,
							"tokenType", si.TokenType,
							"peerID", p.GetPeerID(),
							"peerDID", p.GetPeerDID(),
							"syncResponse", syncResp,
							"err", err,
						)
						syncMu.Lock()
						if syncErr == nil {
							syncErr = fmt.Errorf(
								"Failed to sync token chain for token %s (peer %s, did %s): %v | SyncResponse: %+v",
								si.Token, p.GetPeerID(), p.GetPeerDID(), err, syncResp,
							)
						}
						syncMu.Unlock()
						return
					}
					c.log.Debug("Syncing token chain completed for token", "token", si.Token)
				}(syncInfo)
			}
			
			syncWg.Wait()
			if syncErr != nil {
				return false, syncErr, nil
			}
		}
		
		c.log.Info("Token sync completed", 
			"duration", time.Since(syncStartTime),
			"tokensSynced", len(tokensNeedingSync))
		
		// Process synced tokens and group by block hash
		// IMPORTANT: We must process the tokens BEFORE returning them to the pool
		for i := range tokensNeedingSync {
			syncInfo := &tokensNeedingSync[i]
			
			// Validate token info before processing
			if syncInfo.Token == "" {
				c.log.Error("Empty token found in synced tokens", 
					"index", i, 
					"totalSynced", len(tokensNeedingSync),
					"tokenType", syncInfo.TokenType,
					"blockID", syncInfo.BlockID)
				return false, fmt.Errorf("empty token found at index %d after sync", i), nil
			}
			
			// Get the latest block after sync
			latestBlock := c.w.GetLatestTokenBlock(syncInfo.Token, syncInfo.TokenType)
			if latestBlock == nil {
				// Try genesis block as fallback
				latestBlock = c.w.GetGenesisTokenBlock(syncInfo.Token, syncInfo.TokenType)
				if latestBlock != nil {
					c.log.Info("Using genesis block for token", "token", syncInfo.Token)
				} else {
					// Try to understand why the block is missing
					c.log.Error("Failed to get latest token block after sync", 
						"token", syncInfo.Token,
						"tokenType", syncInfo.TokenType,
						"blockID", syncInfo.BlockID,
						"index", i,
						"totalTokens", len(tokensNeedingSync))
					
					// Check if token exists in wallet
					tokenData, err := c.w.ReadToken(syncInfo.Token)
					if err != nil {
						c.log.Error("Token not found in wallet", "token", syncInfo.Token, "err", err)
					} else {
						c.log.Error("Token exists but no block found", "token", syncInfo.Token, "tokenData", tokenData)
					}
					
					// In trusted network mode, this might be a genesis token
					if c.cfg.CfgData.TrustedNetwork {
						c.log.Warn("In trusted network mode - might be genesis token without chain", "token", syncInfo.Token)
						// Skip this token instead of failing the entire transaction
						c.log.Warn("Skipping token that has no block", "token", syncInfo.Token)
						continue
					}
					
					return false, fmt.Errorf("failed to get latest token block for token %s after sync", syncInfo.Token), nil
				}
			}

			// Get block hash as the grouping key
			blockHash, err := latestBlock.GetHash()
			if err != nil {
				c.log.Error("Failed to get block hash for token", "token", syncInfo.Token, "err", err)
				return false, fmt.Errorf("failed to get block hash for token %s: %v", syncInfo.Token, err), nil
			}

			// Group tokens by block hash
			if result, exists := blockGroups[blockHash]; exists {
				result.Tokens = append(result.Tokens, syncInfo.Token)
			} else {
				blockGroups[blockHash] = &BlockValidationResult{
					BlockHash: blockHash,
					Block:     latestBlock,
					Tokens:    []string{syncInfo.Token},
				}
			}
		}
		
		// Note: We're not using pool for tokensNeedingSync anymore, so no need to return them
	}

	c.log.Info("Token grouping completed", "totalTokens", len(ti), "uniqueBlocks", len(blockGroups))
	c.log.Info("Beginning batch block validation", "uniqueBlocks", len(blockGroups))

	// Log optimization statistics
	batchSynced := 0
	if len(tokensNeedingSync) > 10 {
		batchSynced = len(tokensNeedingSync)
	}
	c.logOptimizationStats(len(ti), len(blockGroups), syncNeeded, syncSkipped, batchSynced)

	// Log grouping statistics
	for blockHash, result := range blockGroups {
		c.log.Debug("Block group", "blockHash", blockHash, "tokenCount", len(result.Tokens))
	}

	// Step 2: Validate each unique block
	var wg sync.WaitGroup
	results := make(chan *BlockValidationResult, len(blockGroups))

	// Use dynamic worker allocation for block validation
	rm := &ResourceMonitor{}
	dynamicWorkers := rm.CalculateDynamicWorkers(len(blockGroups))
	
	// Apply CPU-based limits (numCores * 2)
	numCores := runtime.NumCPU()
	cpuLimit := numCores * 2
	maxWorkers := min(dynamicWorkers, cpuLimit)
	
	c.log.Debug("Block validation worker configuration",
		"uniqueBlocks", len(blockGroups),
		"dynamicWorkers", dynamicWorkers,
		"cpuLimit", cpuLimit,
		"actualWorkers", maxWorkers)
	
	sem := make(chan struct{}, maxWorkers)

	for _, blockResult := range blockGroups {
		wg.Add(1)
		sem <- struct{}{}

		go func(br *BlockValidationResult) {
			defer wg.Done()
			defer func() { <-sem }()

			c.log.Debug("Validating unique block", "blockHash", br.BlockHash, "tokenCount", len(br.Tokens))

			// Validate this block
			valid, err := c.validateSigner(br.Block, quorumDID, p)
			br.IsValid = valid
			br.Error = err

			// Check for sync issues
			if err != nil && strings.Contains(err.Error(), "syncer block height discrepency") {
				br.SyncIssue = true
			}

			results <- br
		}(blockResult)
	}
	wg.Wait()
	close(results)

	// Step 3: Process results and identify failed tokens
	if !c.cfg.CfgData.TrustedNetwork {
		var syncIssueTokens []string
		var failedTokens []string

		for result := range results {
			if !result.IsValid || result.Error != nil {
				c.log.Error("Block validation failed", "blockHash", result.BlockHash, "error", result.Error, "tokenCount", len(result.Tokens))

				if result.SyncIssue {
					// All tokens sharing this block have sync issues
					syncIssueTokens = append(syncIssueTokens, result.Tokens...)
				} else {
					// All tokens sharing this block failed validation
					failedTokens = append(failedTokens, result.Tokens...)
					// Return immediately on first validation failure
					return false, fmt.Errorf("failed to validate token signer for block %s: %v", result.BlockHash, result.Error), nil
				}
			} else {
				c.log.Debug("Block validation successful", "blockHash", result.BlockHash, "tokenCount", len(result.Tokens))
			}
		}
	
		// Step 4: Handle sync issues
		if len(syncIssueTokens) > 0 {
			return false, fmt.Errorf("failed to sync tokenchain Token: issueType: %v", TokenChainNotSynced), syncIssueTokens
		}
	}

	c.log.Info("Optimized token validation completed successfully", 
		"totalTokens", len(ti), 
		"uniqueBlocks", len(blockGroups),
		"tokensSkippedSync", syncSkipped,
		"tokensBatchSynced", batchSynced,
		"validationReduction", fmt.Sprintf("%.1f%%", float64(len(ti)-len(blockGroups))/float64(len(ti))*100))
	return true, nil, nil
}

// validateTokenOwnershipWrapper chooses between optimized and regular validation based on configuration
func (c *Core) validateTokenOwnershipWrapper(cr *ConensusRequest, sc *contract.Contract, quorumDID string) (bool, error, []string) {
	// Check if optimized validation is enabled (you can add this to config later)
	useOptimizedValidation := true // TODO: Make this configurable

	if useOptimizedValidation {
		c.log.Debug("Using optimized token validation")
		return c.validateTokenOwnershipOptimized(cr, sc, quorumDID)
	} else {
		c.log.Debug("Using regular token validation")
		return c.validateTokenOwnership(cr, sc, quorumDID)
	}
}

// logOptimizationStats logs statistics about the optimization
func (c *Core) logOptimizationStats(totalTokens int, uniqueBlocks int, syncNeeded int, syncSkipped int, batchSynced int) {
	reduction := float64(totalTokens-uniqueBlocks) / float64(totalTokens) * 100
	syncReduction := float64(syncSkipped) / float64(totalTokens) * 100
	batchSyncPercent := float64(batchSynced) / float64(totalTokens) * 100
	
	c.log.Info("Token validation optimization stats",
		"totalTokens", totalTokens,
		"uniqueBlocks", uniqueBlocks,
		"reductionPercent", fmt.Sprintf("%.2f%%", reduction),
		"reducedValidations", totalTokens-uniqueBlocks,
		"syncNeeded", syncNeeded,
		"syncSkipped", syncSkipped,
		"syncReductionPercent", fmt.Sprintf("%.2f%%", syncReduction),
		"batchSynced", batchSynced,
		"batchSyncPercent", fmt.Sprintf("%.2f%%", batchSyncPercent))

	// Log optimization breakdown
	c.log.Info("Optimization breakdown",
		"validationOptimization", fmt.Sprintf("%d→%d (%.1f%% reduction)", totalTokens, uniqueBlocks, reduction),
		"syncOptimization", fmt.Sprintf("%d skipped (%.1f%%)", syncSkipped, syncReduction),
		"batchProcessing", fmt.Sprintf("%d tokens (%.1f%%)", batchSynced, batchSyncPercent))

	if reduction > 50 {
		c.log.Info("Significant optimization achieved!", "reduction", fmt.Sprintf("%.2f%%", reduction))
	} else if reduction > 20 {
		c.log.Info("Moderate optimization achieved", "reduction", fmt.Sprintf("%.2f%%", reduction))
	} else {
		c.log.Info("Minimal optimization", "reduction", fmt.Sprintf("%.2f%%", reduction))
	}
}

func (c *Core) validateSignature(dc did.DIDCrypto, h string, s string) bool {
	if dc == nil {
		c.log.Error("Invalid DID setup")
		return false
	}
	sig := util.StrToHex(s)
	ok, err := dc.PvtVerify([]byte(h), sig)
	if err != nil {
		c.log.Error("Error in signature verification", "err", err)
		return false
	}
	if !ok {
		c.log.Error("Failed to verify signature")
		return false
	}
	return true
}

// syncTokensInBatch performs batch token synchronization for improved performance
func (c *Core) syncTokensInBatch(p *ipfsport.Peer, tokens []BatchSyncTokenInfo) error {
	if len(tokens) == 0 {
		return nil
	}

	c.log.Info("Starting batch token sync", "totalTokens", len(tokens))
	startTime := time.Now()

	// Use ResourceMonitor for dynamic worker allocation
	rm := &ResourceMonitor{}
	dynamicWorkers := rm.CalculateDynamicWorkers(len(tokens))
	
	// Cap at 10 workers due to IPFS connection limits
	numWorkers := min(dynamicWorkers, 10)
	
	// Get memory stats for logging
	totalMB, availableMB := rm.GetMemoryStats()
	
	// Use progressive batching for optimal performance
	batchSize := TokenSyncBatchConfig.GetBatchSize(len(tokens))
	
	// Adjust based on memory pressure
	if availableMB < 1000 { // Less than 1GB available
		batchSize = batchSize / 2
		if batchSize < 50 {
			batchSize = 50
		}
	}
	
	c.log.Debug("Using progressive batch size", "batch_size", batchSize, "total_tokens", len(tokens))
	
	c.log.Debug("Batch sync configuration",
		"workers", numWorkers,
		"dynamicWorkers", dynamicWorkers,
		"batchSize", batchSize,
		"totalBatches", (len(tokens)+batchSize-1)/batchSize,
		"totalMemoryMB", totalMB,
		"availableMemoryMB", availableMB)

	// Create batches
	var batches [][]BatchSyncTokenInfo
	for i := 0; i < len(tokens); i += batchSize {
		end := min(i+batchSize, len(tokens))
		batches = append(batches, tokens[i:end])
	}

	// Sync batches with controlled concurrency
	errChan := make(chan error, len(batches))
	sem := make(chan struct{}, numWorkers) // Limit to 10 concurrent operations
	
	var wg sync.WaitGroup
	for batchIdx, batch := range batches {
		wg.Add(1)
		go func(idx int, tokenBatch []BatchSyncTokenInfo) {
			defer wg.Done()
			
			sem <- struct{}{} // Acquire semaphore
			defer func() { <-sem }() // Release semaphore
			
			c.log.Debug("Processing batch", "batchIndex", idx, "batchSize", len(tokenBatch))
			
			// Sync each token in the batch
			for _, tokenInfo := range tokenBatch {
				err, syncResp := c.syncTokenChainFrom(p, tokenInfo.BlockID, tokenInfo.Token, tokenInfo.TokenType)
				if err != nil {
					c.log.Error("Failed to sync token in batch",
						"token", tokenInfo.Token,
						"batch", idx,
						"err", err,
						"syncResponse", syncResp)
					errChan <- fmt.Errorf("failed to sync token %s: %v", tokenInfo.Token, err)
					return
				}
			}
			
			c.log.Debug("Batch sync completed", "batchIndex", idx, "tokensProcessed", len(tokenBatch))
		}(batchIdx, batch)
	}
	
	wg.Wait()
	close(errChan)
	
	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("batch sync failed with %d errors: %v", len(errors), errors[0])
	}
	
	duration := time.Since(startTime)
	tokensPerSecond := float64(len(tokens)) / duration.Seconds()
	avgBatchTime := duration.Milliseconds() / int64(len(batches))
	
	c.log.Info("Batch token sync completed successfully",
		"totalTokens", len(tokens),
		"batches", len(batches),
		"workers", numWorkers,
		"duration", duration,
		"tokensPerSecond", fmt.Sprintf("%.2f", tokensPerSecond),
		"avgBatchTimeMs", avgBatchTime,
		"memoryUsedMB", totalMB-availableMB)
	
	return nil
}



// func (c *Core) checkTokenIsPledged(wt string) bool {
// 	tokenType := token.RBTTokenType
// 	if c.testNet {
// 		tokenType = token.TestTokenType
// 	}
// 	b := c.w.GetLatestTokenBlock(wt, tokenType)
// 	if b == nil {
// 		c.log.Error("Invalid token chain block")
// 		return true
// 	}
// 	return c.checkIsPledged(b, wt)
// }

// func (c *Core) checkTokenIsUnpledged(wt string) bool {
// 	tokenType := token.RBTTokenType
// 	if c.testNet {
// 		tokenType = token.TestTokenType
// 	}
// 	b := c.w.GetLatestTokenBlock(wt, tokenType)
// 	if b == nil {
// 		c.log.Error("Invalid token chain block")
// 		return true
// 	}
// 	return c.checkIsUnpledged(b, wt)
// }

func (c *Core) getUnpledgeId(wt string, tokenType int) string {
	b := c.w.GetLatestTokenBlock(wt, tokenType)
	if b == nil {
		c.log.Error("Invalid token chain block")
		return ""
	}
	return b.GetUnpledgeId(wt)
}

/*
 * Function to check whether the TokenState is pinned or not
 * Input tokenId, index, resultArray, waitgroup,quorumList
 */
func (c *Core) checkTokenState(tokenId, did string, index int, resultArray []TokenStateCheckResult, quorumList []string, tokenType int) {
	// Track individual token state check
	defer c.TrackOperation("token.state_check.single_token", map[string]interface{}{
		"token": tokenId,
		"token_type": tokenType,
		"index": index,
	})(nil)
	
	var result TokenStateCheckResult
	result.Token = tokenId

	//get the latest blockId i.e. latest token state
	block := c.w.GetLatestTokenBlock(tokenId, tokenType)
	if block == nil {
		c.log.Error("Invalid token chain block, Block is nil")
		result.Error = fmt.Errorf("Invalid token chain block,Block is nil")
		result.Message = "Invalid token chain block"
		resultArray[index] = result
		return
	}
	blockId, err := block.GetBlockID(tokenId)
	if err != nil {
		c.log.Error("Error fetching block Id", err)
		result.Error = err
		result.Message = "Error fetching block Id"
		resultArray[index] = result
		return
	}
	//concat tokenId and BlockID
	tokenIDTokenStateData := tokenId + blockId
	tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenIDTokenStateData))

	//add to ipfs get only the hash of the token+tokenstate
	tokenIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
	result.tokenIDTokenStateHash = tokenIDTokenStateHash
	if err != nil {
		c.log.Error("Error adding data to ipfs", err)
		result.Error = err
		result.Message = "Error adding data to ipfs"
		resultArray[index] = result
		return
	}

	//check to see if tokenstate was already pinned by current validator, for any previous consensus
	tokenStatePinInfo, err := c.w.GetStatePinnedInfo(tokenIDTokenStateHash)
	if err != nil {
		c.log.Error("Error checking if tokenstate pinned earlier", err)
		result.Error = err
		result.Message = "Error checking if tokenstate pinned earlier"
		resultArray[index] = result
		return
	}

	if tokenStatePinInfo != nil {
		c.log.Debug("Tokenstate pinned already pinned", err)
		result.Error = err
		result.Message = "Tokenstate pinned already pinned"
		resultArray[index] = result
		return
	}

	// Skip DHT check in trusted network mode
	if c.cfg.CfgData.TrustedNetwork {
		c.log.Debug("Skipping DHT check in trusted network mode", "token", tokenId)
	} else {
		//check dht to see if any pin exist
		list, err1 := c.GetDHTddrs(tokenIDTokenStateHash)
		//try to call ipfs cat to check if any one has pinned the state i.e \
		if err1 != nil {
			c.log.Error("Error fetching content for the tokenstate ipfs hash :", tokenIDTokenStateHash, "Error", err)
			result.Exhausted = true
			result.Error = nil
			result.Message = "Error fetching content for the tokenstate ipfs hash : " + tokenIDTokenStateHash
			resultArray[index] = result
			return
		}
		//remove ql peer ids from list
		qPeerIds := make([]string, 0)

		for i := range quorumList {
			pId, _, ok := util.ParseAddress(quorumList[i])
			if !ok {
				c.log.Error("Error parsing addressing")
				result.Error = err
				result.Message = "Error parsing addressing"
				resultArray[index] = result
				return
			}
			qPeerIds = append(qPeerIds, pId)
		}
		c.log.Debug("Quorum Peer IDs to remove", qPeerIds)
		c.log.Debug("List of Peer IDs from DHT", list)
		c.log.Debug("did in input", did)
		peerId := c.w.GetPeerID(did)
		if peerId == "" {
			c.log.Error("Peer ID not found for DID", did)
		} else {
			c.log.Debug("Peer ID found for DID", did, peerId)
			qPeerIds = append(qPeerIds, peerId)
		}

		updatedList := c.removeStrings(list, qPeerIds)
		c.log.Debug("Updated List after removing quorum peer ids", updatedList)
		c.log.Debug("len of updated list", len(updatedList))
		//if pin exist abort
		if len(updatedList) > 1 {
			c.log.Debug("Token state is exhausted, Token is being Double spent. Token : ", tokenId)
			result.Exhausted = true
			result.Error = nil
			result.Message = "Token state is exhausted, Token is being Double spent. Token : " + tokenId
			resultArray[index] = result
			return
		}
	}

	// Don't log for each token to reduce noise
	result.Error = nil
	result.Message = "Token state is free, Unique Txn"
	result.tokenIDTokenStateData = tokenIDTokenStateData
	resultArray[index] = result
}

func (c *Core) pinTokenState(
	ctx context.Context,
	tokenStateCheckResult []TokenStateCheckResult,
	did, transactionId, sender, receiver string,
	tokenValue float64,
) error {
	total := len(tokenStateCheckResult)

	// For very large token counts, add initial delay to let quorums stabilize
	if total > 500 {
		stabilizationDelay := time.Duration(total/500) * time.Second
		c.log.Info("Adding stabilization delay for large transaction",
			"tokens", total,
			"delay", stabilizationDelay)
		time.Sleep(stabilizationDelay)
	}

	// Set up memory optimization for large operations
	memOptimizer := NewMemoryOptimizer(c.log)
	memOptimizer.OptimizeForLargeOperation(total)
	defer memOptimizer.RestoreDefaults()

	// Start memory monitoring
	monitorDone := make(chan struct{})
	go memOptimizer.MonitorMemoryPressure(monitorDone)
	defer close(monitorDone)

	// Start periodic GC for large operations
	if total > 100 {
		gcDone := make(chan struct{})
		go memOptimizer.PeriodicGC(gcDone, 30*time.Second)
		defer close(gcDone)
	}

	var (
		ids              []string
		completed        int32
		lastLoggedPct    int32
		mu               sync.Mutex // Protects shared slice `ids`
		providerMapMutex sync.Mutex // Protects providerMaps
		wg               sync.WaitGroup
		errOnce          sync.Once
		firstErr         error
		tasks            = make(chan int, total)
		cancelableCtx, _ = context.WithCancel(ctx) // In case you want to cancel all on first error
		providerMaps     = make([]model.TokenProviderMap, 0, total)
	)

	// Use dynamic worker sizing based on available resources
	rm := &ResourceMonitor{}
	numWorkers := rm.CalculateDynamicWorkers(total)

	// Log resource stats for monitoring
	stats := rm.GetResourceStats()
	c.log.Info("Resource-based worker pool sizing",
		"tokens", total,
		"workers", numWorkers,
		"memory_available_mb", stats["memory_available_mb"],
		"memory_usage_pct", stats["memory_usage_pct"],
		"goroutines", stats["goroutines"])

	if total == 0 {
		c.log.Warn("No token states to pin")
		return nil
	}

	// Worker pool
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				select {
				case <-cancelableCtx.Done():
					return
				default:
				}

				data := tokenStateCheckResult[i].tokenIDTokenStateData

				var tokenIDTokenStateHash string
				var tpm model.TokenProviderMap
				var err error

				// Retry block for AddWithProviderMap
				err = retry(func() error {
					var retryErr error
					tokenIDTokenStateHash, tpm, retryErr = c.w.AddWithProviderMap(
						bytes.NewBuffer([]byte(data)),
						did,
						wallet.QuorumPinRole,
					)
					// Fill in extra fields for pinning
					tpm.FuncID = wallet.PinFunc
					tpm.TransactionID = transactionId
					tpm.Sender = sender
					tpm.Receiver = receiver
					tpm.TokenValue = tokenValue
					return retryErr
				})
				if err != nil {
					c.log.Error("Failed to add token state after retries", "index", i, "err", err)
					recordFirstError(&firstErr, err, &errOnce)
					return
				}

				// Save the hash safely
				mu.Lock()
				ids = append(ids, tokenIDTokenStateHash)
				mu.Unlock()

				// Add small delay for IPFS stability with large token counts
				if total > 500 {
					time.Sleep(50 * time.Millisecond) // 50ms between pins for large batches
				} else if total > 250 {
					time.Sleep(25 * time.Millisecond) // 25ms for medium batches
				}

				// Retry block for Pin (but skip AddProviderDetails inside Pin)
				err = retry(func() error {
					_, retryErr := c.w.Pin(tokenIDTokenStateHash, wallet.QuorumPinRole, did, transactionId, sender, receiver, tokenValue, true)
					return retryErr
				})
				if err != nil {
					c.log.Error("Failed to pin token state after retries", "index", i, "err", err)
					recordFirstError(&firstErr, err, &errOnce)
					// Optionally unpin already pinned
					if unpinErr := c.unPinTokenState(ids, did); unpinErr != nil {
						c.log.Warn("Failed to unpin token states after pin failure", "err", unpinErr)
					}
					return
				}

				// Collect provider map for batch
				providerMapMutex.Lock()
				providerMaps = append(providerMaps, tpm)
				providerMapMutex.Unlock()

				newCount := atomic.AddInt32(&completed, 1)
				currentPct := int32(math.Floor(float64(newCount*100) / float64(total)))
				if currentPct%10 == 0 && atomic.LoadInt32(&lastLoggedPct) < currentPct {
					if atomic.CompareAndSwapInt32(&lastLoggedPct, lastLoggedPct, currentPct) {
						c.log.Debug(fmt.Sprintf("Pinning progress: %d%% (%d/%d)", currentPct, newCount, total))
					}
				}

				c.log.Debug("Token state pinned", "hash", tokenIDTokenStateHash)

				// Add processing rate control for large batches
				if total > 100 {
					// Calculate delay based on worker count and progress
					var processingDelay time.Duration
					if numWorkers <= 2 {
						processingDelay = 200 * time.Millisecond // Slow rate for few workers
					} else if numWorkers <= 4 {
						processingDelay = 100 * time.Millisecond // Medium rate
					} else {
						processingDelay = 50 * time.Millisecond // Faster rate for many workers
					}

					// Add jitter to prevent thundering herd
					jitter := time.Duration(i%10) * 10 * time.Millisecond
					time.Sleep(processingDelay + jitter)
				}
			}
		}()
	}

	// Enqueue tasks
	for i := 0; i < total; i++ {
		tasks <- i
	}
	close(tasks)

	wg.Wait()

	if firstErr != nil {
		return fmt.Errorf("pinning failed: %w", firstErr)
	}

	// Batch write provider details with retry/backoff
	// Note: AddProviderDetailsBatch now handles UNIQUE constraints gracefully
	err := c.w.AddProviderDetailsBatch(providerMaps)
	if err != nil {
		// Log the error but don't fail the validation
		// UNIQUE constraint errors are expected when multiple quorums process the same tokens
		c.log.Warn("Failed to add some provider details", "err", err, "total_tokens", len(providerMaps))
		if strings.Contains(err.Error(), "all provider detail operations failed") {
			// Only fail if ALL operations failed
			return fmt.Errorf("critical error in provider details: %w", err)
		}
	}

	c.log.Debug("Provider details batch completed", "total_tokens", len(providerMaps))
	return nil
}

func (c *Core) unPinTokenState(ids []string, did string) error {
	for _, id := range ids {
		_, err := c.w.UnPin(id, wallet.QuorumRole, did)
		if err != nil {
			c.log.Warn("Error unpinning token state", "id", id, "err", err)
			return err
		}
	}
	return nil
}
