package core

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

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

func (c *Core) validateTokenOwnership(cr *ConensusRequest, sc *contract.Contract, quorumDID string) (bool, error) {

	var ti []contract.TokenInfo
	var address string
	var receiverAddress string
	if cr.Mode == SmartContractDeployMode || cr.Mode == NFTDeployMode {
		ti = sc.GetCommitedTokensInfo()
		address = cr.DeployerPeerID + "." + sc.GetDeployerDID()
	} else {
		ti = sc.GetTransTokenInfo()
		address = cr.SenderPeerID + "." + sc.GetSenderDID()
		receiverAddress = cr.ReceiverPeerID + "." + sc.GetReceiverDID()
	}
	for i := range ti {
		ids, err := c.GetDHTddrs(ti[i].Token)
		if err != nil || len(ids) == 0 {
			continue
		}
	}
	p, err := c.getPeer(address)
	if err != nil {
		c.log.Error("Failed to get peer", "err", err)
		return false, err
	}
	defer p.Close()
	tokensSyncInfo := make([]TokenSyncInfo, 0)
	for i := range ti {
		err := c.syncTokenChainFrom(p, ti[i].BlockID, ti[i].Token, ti[i].TokenType)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			return false, fmt.Errorf("failed to sync tokenchain Token: %v, issueType: %v", ti[i].Token, TokenChainNotSynced)
		}
		// blocksToSync := cr.TransTokenSyncInfo[ti[i].Token]
		// genesisBlock := block.InitBlock(blocksToSync.GenesisBlock, nil)
		// if genesisBlock == nil {
		// 	c.log.Error("Failed to add genesis block, invalid block of token", ti[i].Token)
		// 	return false, fmt.Errorf("Failed to add genesis block, invalid block of token : %v", ti[i].Token)
		// }
		// err := c.w.AddTokenBlock(ti[i].Token, genesisBlock)
		// if err != nil {
		// 	c.log.Error("Failed to add genesis block of token", ti[i].Token, "err", err)
		// 	return false, err
		// }

		// if blocksToSync.LatestBlock != nil {
		// 	latestBlock := block.InitBlock(blocksToSync.LatestBlock, nil)
		// 	err := c.w.AddTokenBlock(ti[i].Token, latestBlock)
		// 	if err != nil {
		// 		c.log.Error("Failed to add last token chain block of token", ti[i].Token, "err", err)
		// 		return false, err
		// 	}
		// }

		genesisBlock := c.w.GetGenesisTokenBlock(ti[i].Token, ti[i].TokenType)
		if genesisBlock == nil {
			// p.Close()
			c.log.Error("Failed to get first token chain block")
			return false, fmt.Errorf("failed to get first token chain block %v", ti[i].Token)
		}
		var parentToken string
		if c.TokenType(PartString) == ti[i].TokenType {
			parentToken, _, err := genesisBlock.GetParentDetials(ti[i].Token)
			if err != nil {
				// p.Close()
				c.log.Error("failed to fetch parent token detials", "err", err, "token", ti[i].Token)
				return false, err
			}
			parentTokenType, err := c.syncParentToken(p, parentToken)
			if err != nil {
				// p.Close()
				c.log.Error("failed to sync parent token chain", "token", parentToken)
				return false, err
			}
			tokensSyncInfo = append(tokensSyncInfo, TokenSyncInfo{TokenID: parentToken, TokenType: parentTokenType})

			// // add parent blocks
			// parentGenesisBlock := block.InitBlock(blocksToSync.ParentGenesisBlock, nil)
			// if parentGenesisBlock == nil {
			// 	c.log.Error("Failed to add genesis block, invalid block of parent token", pt, "token", ti[i].Token)
			// 	return false, fmt.Errorf("failed to add parent genesis block, invalid parent genesis block of token : %v", ti[i].Token)
			// }
			// err = c.w.AddTokenBlock(pt, parentGenesisBlock)
			// if err != nil {
			// 	c.log.Error("Failed to add parent's genesis block of token", ti[i].Token, "err", err)
			// 	return false, err
			// }

			// parentLatestBlock := block.InitBlock(blocksToSync.ParentLatestBlock, nil)
			// err = c.w.AddTokenBlock(pt, parentLatestBlock)
			// if err != nil {
			// 	c.log.Error("Failed to add parent's latest token chain block of token", ti[i].Token, "err", err)
			// 	return false, err
			// }
			_, err = c.w.Pin(parentToken, wallet.ParentTokenPinByQuorumRole, quorumDID, cr.TransactionID, address, receiverAddress, ti[i].TokenValue)
			if err != nil {
				// p.Close()
				c.log.Error("Failed to Pin parent token in Quorum", "err", err)
				return false, err
			}
		}

		// Check the token validation
		if ti[i].TokenType == token.RBTTokenType {
			tl, tn, err := genesisBlock.GetTokenDetials(ti[i].Token)
			if err != nil {
				// p.Close()
				c.log.Error("Failed to get token detials", "err", err)
				return false, err
			}
			ct := token.GetTokenString(tl, tn)
			tb := bytes.NewBuffer([]byte(ct))
			tid, err := c.ipfs.Add(tb, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				// p.Close()
				c.log.Error("Failed to validate, failed to get token hash", "err", err)
				return false, err
			}
			if tid != ti[i].Token {
				// p.Close()
				c.log.Error("Invalid token", "token", ti[i].Token, "exp_token", tid, "tl", tl, "tn", tn)
				return false, fmt.Errorf("Invalid token", "token", ti[i].Token, "exp_token", tid, "tl", tl, "tn", tn)
			}
		}
		b := c.w.GetLatestTokenBlock(ti[i].Token, ti[i].TokenType)
		if b == nil {
			// p.Close()
			c.log.Error("Invalid token chain block")
			return false, fmt.Errorf("Invalid token chain block for ", ti[i].Token)
		}
		c.log.Info("Validating token ownership", "token", ti[i].Token, "owner", b.GetOwner(), "sender", sc.GetSenderDID())
		pinningNodeDID := b.GetPinningNodeDID()
		ownerDID := b.GetOwner()
		senderDID := sc.GetSenderDID()

		if pinningNodeDID != "" {
			c.log.Info("The token is Pinned as a service on Node ", pinningNodeDID)
			if ownerDID != senderDID {
				// p.Close()
				c.log.Error("Invalid token owner: The token is Pinned as a service", "owner", ownerDID, "The node which is trying to transfer", senderDID)
				return false, fmt.Errorf("invalid token owner: The token is Pinned as a service")
			}
		}
		signatureValidation, err := c.validateSigner(b, quorumDID, p)
		if !signatureValidation || err != nil {
			// p.Close()
			c.log.Error("Failed to validate token ownership ", "token ID:", ti[i].Token)
			return false, err
		}

		// add trans tokens to TokensTable with token status = 17
		// Check if token already exists
		tokenInfo, err := c.w.ReadToken(ti[i].Token)
		if err != nil || tokenInfo.TokenID == "" {
			// // Token doesn't exist, proceed to handle it
			// dir := util.GetRandString()
			// if err := util.CreateDir(dir); err != nil {
			// 	c.log.Error("Failed to create directory", "err", err)
			// 	return false, err
			// }
			// defer os.RemoveAll(dir)

			// // Get the token
			// if err := w.Get(tokenInfo.Token, did, OwnerRole, dir); err != nil {
			// 	w.log.Error("Failed to get token", "err", err)
			// 	return nil, err
			// }

			// Create new token entry
			tokenInfo = &wallet.Token{
				TokenID:       ti[i].Token,
				TokenValue:    ti[i].TokenValue,
				ParentTokenID: parentToken,
				DID:           ti[i].OwnerDID,
			}

			err = c.w.CreateToken(tokenInfo)
			if err != nil {
				c.log.Error("failed to write to db, token ", ti[i].Token, "err", err)
				return false, err
			}
		}

		// Update token status
		tokenInfo.DID = senderDID
		tokenInfo.TokenStatus = wallet.QuorumPledgedForThisToken
		tokenInfo.TransactionID = b.GetTid()

		// t.TokenStateHash = tokenHashMap[ti[i].Token]
		tokenInfo.SyncStatus = wallet.SyncIncomplete

		err = c.w.UpdateToken(tokenInfo)
		if err != nil {
			c.log.Error("failed to update to db, token ", ti[i].Token, "err", err)
			return false, err
		}

		// quorum fetches tokens to be synced
		tokensSyncInfo = append(tokensSyncInfo, TokenSyncInfo{TokenID: ti[i].Token, TokenType: ti[i].TokenType})
	}

	// sync full token chain of all the tokens in syncing Queue
	tokenSyncMap := make(map[string][]TokenSyncInfo)
	tokenSyncMap[p.GetPeerID()+"."+p.GetPeerDID()] = tokensSyncInfo
	go c.syncFullTokenChains(tokenSyncMap)

	// for i := range wt {
	// 	c.log.Debug("Requesting Token status")
	// 	ts := TokenPublish{
	// 		Token: wt[i],
	// 	}
	// 	c.ps.Publish(TokenStatusTopic, &ts)
	// 	c.log.Debug("Finding dht", "token", wt[i])
	// 	ids, err := c.GetDHTddrs(wt[i])
	// 	if err != nil || len(ids) == 0 {
	// 		c.log.Error("Failed to find token owner", "err", err)
	// 		crep.Message = "Failed to find token owner"
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	}
	// 	if len(ids) > 1 {
	// 		// ::TODO:: to more check to findout right pwner
	// 		c.log.Error("Mutiple owner found for the token", "token", wt, "owners", ids)
	// 		crep.Message = "Mutiple owner found for the token"
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	} else {
	// 		//:TODO:: get peer from the table
	// 		if cr.SenderPeerID != ids[0] {
	// 			c.log.Error("Token peer id mismatched", "expPeerdID", cr.SenderPeerID, "peerID", ids[0])
	// 			crep.Message = "Token peer id mismatched"
	// 			return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 		}
	// 	}
	// }
	return true, nil
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
func (c *Core) checkTokenState(tokenId, did string, index int, resultArray []TokenStateCheckResult, wg *sync.WaitGroup, quorumList []string, tokenType int) {
	defer wg.Done()
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
	tokenIDTokenStateHash, err := c.ipfs.Add(tokenIDTokenStateBuffer, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
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

func (c *Core) pinTokenState(tokenStateCheckResult []TokenStateCheckResult, did string, transactionId string, sender string, receiver string, tokenValue float64) error {
	var ids []string
	for i := range tokenStateCheckResult {
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenStateCheckResult[i].tokenIDTokenStateData))
		tokenIDTokenStateHash, err := c.w.Add(tokenIDTokenStateBuffer, did, wallet.QuorumPinRole)
		if err != nil {
			c.log.Error("Error triggered while adding token state", err)
			return err
		}
		ids = append(ids, tokenIDTokenStateHash)
		_, err = c.w.Pin(tokenIDTokenStateHash, wallet.QuorumPinRole, did, transactionId, sender, receiver, tokenValue)
		if err != nil {
			c.log.Error("Error triggered while pinning token state", err)
			c.unPinTokenState(ids, did)
			return err
		}
		c.log.Debug("token state pinned", tokenIDTokenStateHash)
	}
	return nil
}

func (c *Core) unPinTokenState(ids []string, did string) {
	for i := range ids {
		c.w.UnPin(ids[i], wallet.QuorumRole, did)
	}
}
