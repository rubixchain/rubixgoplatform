package core

import (
	"bytes"
	"fmt"
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
}

func (c *Core) validateSigner(b *block.Block) bool {
	signers, err := b.GetSigner()
	if err != nil {
		c.log.Error("failed to get signers", "err", err)
		return false
	}
	for _, signer := range signers {
		var dc did.DIDCrypto
		switch b.GetTransType() {
		case block.TokenGeneratedType, block.TokenBurntType:
			dc, err = c.SetupForienDID(signer)
			if err != nil {
				c.log.Error("failed to setup forien DID", "err", err)
				return false
			}
		default:
			dc, err = c.SetupForienDIDQuorum(signer)
			if err != nil {
				c.log.Error("failed to setup forien DID quorum", "err", err)
				return false
			}
		}
		err := b.VerifySignature(dc)
		if err != nil {
			c.log.Error("Failed to verify signature", "err", err)
			return false
		}
	}
	return true
}

func (c *Core) syncParentToken(p *ipfsport.Peer, pt string) error {
	b, err := c.getFromIPFS(pt)
	if err != nil {
		c.log.Error("failed to get parent token detials from ipfs", "err", err, "token", pt)
		return err
	}
	_, _, ok, err := token.GetWholeTokenValue(string(b))
	tt := token.RBTTokenType
	tv := float64(1)
	if err == nil || !ok {
		blk := util.StrToHex(string(b))
		rb, err := rac.InitRacBlock(blk, nil)
		if err != nil {
			c.log.Error("invalid token, invalid rac block", "err", err)
			return err
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
		return err
	}
	ptb := c.w.GetLatestTokenBlock(pt, tt)
	if ptb == nil {
		c.log.Error("Failed to get latest token chain block", "token", pt)
		return fmt.Errorf("failed to get latest block")
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
				return fmt.Errorf("failed to get genesis token chain block")
			}
			ppt, _, err := gb.GetParentDetials(pt)
			if err != nil {
				c.log.Error("failed to get genesis token chain block", "token", pt, "err", err)
				return fmt.Errorf("failed to get genesis token chain block")
			}
			td.ParentTokenID = ppt
		}
		c.w.CreateToken(td)
	} else {
		td.TokenStatus = wallet.TokenIsBurnt
		c.w.UpdateToken(td)
	}
	if ptb.GetTransType() != block.TokenBurntType {
		c.log.Error("parent token is not in burnt stage", "token", pt)
		return fmt.Errorf("parent token is not in burnt stage")
	}
	return nil
}

func (c *Core) validateTokenOwnership(cr *ConensusRequest, sc *contract.Contract) bool {

	var ti []contract.TokenInfo
	var address string
	if cr.Mode == SmartContractDeployMode {
		ti = sc.GetCommitedTokensInfo()
		address = cr.DeployerPeerID + "." + sc.GetDeployerDID()
	} else {
		ti = sc.GetTransTokenInfo()
		address = cr.SenderPeerID + "." + sc.GetSenderDID()
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
		return false
	}
	defer p.Close()
	for i := range ti {
		err := c.syncTokenChainFrom(p, ti[i].BlockID, ti[i].Token, ti[i].TokenType)
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			return false
		}
		fb := c.w.GetGenesisTokenBlock(ti[i].Token, ti[i].TokenType)
		if fb == nil {
			c.log.Error("Failed to get first token chain block")
			return false
		}
		if c.TokenType(PartString) == ti[i].TokenType {
			pt, _, err := fb.GetParentDetials(ti[i].Token)
			if err != nil {
				c.log.Error("failed to fetch parent token detials", "err", err, "token", ti[i].Token)
				return false
			}
			err = c.syncParentToken(p, pt)
			if err != nil {
				c.log.Error("failed to sync parent token chain", "token", pt)
				return false
			}
		}

		// Check the token validation
		if ti[i].TokenType == token.RBTTokenType {
			tl, tn, err := fb.GetTokenDetials(ti[i].Token)
			if err != nil {
				c.log.Error("Failed to get token detials", "err", err)
				return false
			}
			ct := token.GetTokenString(tl, tn)
			tb := bytes.NewBuffer([]byte(ct))
			tid, err := c.ipfs.Add(tb, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
			if err != nil {
				c.log.Error("Failed to validate, failed to get token hash", "err", err)
				return false
			}
			if tid != ti[i].Token {
				c.log.Error("Invalid token", "token", ti[i].Token, "exp_token", tid, "tl", tl, "tn", tn)
				return false
			}
		}
		b := c.w.GetLatestTokenBlock(ti[i].Token, ti[i].TokenType)
		if b == nil {
			c.log.Error("Invalid token chain block")
			return false
		}
		if !c.validateSigner(b) {
			return false
		}
	}
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
	return true
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

func (c *Core) getUnpledgeId(wt string) string {
	tokenType := token.RBTTokenType
	if c.testNet {
		tokenType = token.TestTokenType
	}
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
		c.log.Debug("Token state is exhausted, Token is being Double spent")
		result.Exhausted = true
		result.Error = nil
		result.Message = "Token state is exhausted, Token is being Double spent"
		resultArray[index] = result
		return
	}
	c.log.Debug("Token state is not exhausted, Unique Txn")
	result.Error = nil
	result.Message = "Token state is free, Unique Txn"
	result.tokenIDTokenStateData = tokenIDTokenStateData
	resultArray[index] = result
}

func (c *Core) pinTokenState(tokenStateCheckResult []TokenStateCheckResult, did string) error {
	var ids []string
	for i := range tokenStateCheckResult {
		tokenIDTokenStateBuffer := bytes.NewBuffer([]byte(tokenStateCheckResult[i].tokenIDTokenStateData))
		tokenIDTokenStateHash, err := c.w.Add(tokenIDTokenStateBuffer, did, wallet.QuorumRole)
		if err != nil {
			c.log.Error("Error triggered while adding token state", err)
			return err
		}
		ids = append(ids, tokenIDTokenStateHash)
		_, err = c.w.Pin(tokenIDTokenStateHash, wallet.QuorumRole, did)
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
