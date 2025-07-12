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
	err = c.syncTokenChainFrom(p, lbID, pt, tt)
	if err != nil {
		c.log.Error("failed to sync token chain block", "err", err)
		return -1, fmt.Errorf("failed to sync tokenchain Parent Token: %v, issueType: %v", pt, TokenChainNotSynced)
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
	if ids, err := c.GetDHTddrs(ti.Token); err != nil || len(ids) == 0 {
		return nil, false // skip token if no DHT entries found
	}

	if err := c.syncTokenChainFrom(p, ti.BlockID, ti.Token, ti.TokenType); err != nil {
		if strings.Contains(err.Error(), "syncer block height discrepency") {
			// logic for handling block height discrepancy if needed
			return nil, true // mark this token as having a sync issue
		}
		return err, true
	}

	genesisBlock := c.w.GetGenesisTokenBlock(ti.Token, ti.TokenType)
	if genesisBlock == nil {
		return fmt.Errorf("failed to get first token chain block %v", ti.Token), false
	}

	if c.TokenType(PartString) == ti.TokenType {
		parentToken, _, err := genesisBlock.GetParentDetials(ti.Token)
		if err != nil {
			return err, false
		}
		parentTokenType, err := c.syncParentToken(p, parentToken)
		if err != nil || parentTokenType == -1 {
			return err, false
		}
		_, err = c.w.Pin(parentToken, wallet.ParentTokenPinByQuorumRole, quorumDID, cr.TransactionID, address, receiverAddress, ti.TokenValue)
		if err != nil {
			return err, false
		}
	}

	if ti.TokenType == token.RBTTokenType {
		tl, tn, err := genesisBlock.GetTokenDetials(ti.Token)
		if err != nil {
			return err, false
		}
		tid, err := IpfsAddWithBackoff(c.ipfs, bytes.NewBufferString(token.GetTokenString(tl, tn)), ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if err != nil {
			return err, false
		}
		if tid != ti.Token {
			return fmt.Errorf("Invalid token hash for %s", ti.Token), false
		}
	}

	b := c.w.GetLatestTokenBlock(ti.Token, ti.TokenType)
	if b == nil {
		return fmt.Errorf("Invalid token chain block for %s", ti.Token), false
	}

	if b.GetPinningNodeDID() != "" && b.GetOwner() != sc.GetSenderDID() {
		return fmt.Errorf("invalid token owner: token pinned as service, token %s", ti.Token), false
	}

	valid, err := c.validateSigner(b, quorumDID, p)
	if !valid || err != nil {
		return fmt.Errorf("failed to validate token signer for %s", ti.Token), false
	}

	tokenInfo, err := c.w.ReadToken(ti.Token)
	if err != nil || tokenInfo.TokenID == "" {
		tokenInfo = &wallet.Token{
			TokenID:       ti.Token,
			TokenValue:    ti.TokenValue,
			ParentTokenID: "",
			DID:           ti.OwnerDID,
		}
		dbWriteSem <- struct{}{}
		err := util.RetrySQLiteWrite(func() error {
			return c.w.CreateToken(tokenInfo)
		}, 10, 100*time.Millisecond)
		<-dbWriteSem
		if err != nil {
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
			err, syncIssue := c.validateSingleToken(cr, sc, quorumDID, t, p, address, receiverAddress)
			results <- tokenValidationResult{
				Token:     t.Token,
				Err:       err,
				SyncIssue: syncIssue,
			}
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
	updatedList := c.removeStrings(list, qPeerIds)
	//if pin exist abort
	if len(updatedList) != 0 {
		c.log.Debug("Token state is exhausted, Token is being Double spent. Token : ", tokenId)
		result.Exhausted = true
		result.Error = nil
		result.Message = "Token state is exhausted, Token is being Double spent. Token : " + tokenId
		resultArray[index] = result
		return
	}

	c.log.Debug("Token state is not exhausted, Unique Txn")
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
	var (
		ids              []string
		total            = len(tokenStateCheckResult)
		completed        int32
		lastLoggedPct    int32
		mu               sync.Mutex // Protects shared slice `ids`
		wg               sync.WaitGroup
		errOnce          sync.Once
		firstErr         error
		numWorkers       = runtime.NumCPU()
		tasks            = make(chan int, total)
		cancelableCtx, _ = context.WithCancel(ctx) // In case you want to cancel all on first error
	)

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
				var err error

				// Retry block for Add
				err = retry(func() error {
					var retryErr error
					tokenIDTokenStateHash, retryErr = c.w.Add(
						bytes.NewBuffer([]byte(data)),
						did,
						wallet.QuorumPinRole,
					)
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

				// Retry block for Pin
				err = retry(func() error {
					_, retryErr := c.w.Pin(
						tokenIDTokenStateHash,
						wallet.QuorumPinRole,
						did,
						transactionId,
						sender,
						receiver,
						tokenValue,
					)
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

				newCount := atomic.AddInt32(&completed, 1)
				currentPct := int32(math.Floor(float64(newCount*100) / float64(total)))
				if currentPct%10 == 0 && atomic.LoadInt32(&lastLoggedPct) < currentPct {
					if atomic.CompareAndSwapInt32(&lastLoggedPct, lastLoggedPct, currentPct) {
						c.log.Debug(fmt.Sprintf("Pinning progress: %d%% (%d/%d)", currentPct, newCount, total))
					}
				}

				c.log.Debug("Token state pinned", "hash", tokenIDTokenStateHash)
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
