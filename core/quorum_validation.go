package core

import (
	"bytes"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) validateTokenOwnership(cr *ConensusRequest, sc *contract.Contract) bool {
	ti := sc.GetTransTokenInfo()
	for i := range ti {
		ids, err := c.GetDHTddrs(ti[i].Token)
		if err != nil || len(ids) == 0 {
			continue
		}
	}
	address := cr.SenderPeerID + "." + sc.GetSenderDID()
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
		// Check the token validation
		if !c.testNet {
			fb := c.W.GetFirstBlock(ti[i].Token, ti[i].TokenType)
			if fb == nil {
				c.log.Error("Failed to get first token chain block")
				return false
			}
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
		b := c.W.GetLatestTokenBlock(ti[i].Token, ti[i].TokenType)
		if b == nil {
			c.log.Error("Invalid token chain block")
			return false
		}
		signers, err := b.GetSigner()
		if err != nil {
			c.log.Error("Failed to signers", "err", err)
			return false
		}
		for _, signer := range signers {
			var dc did.DIDCrypto
			switch b.GetTransType() {
			case block.TokenGeneratedType:
				dc, err = c.SetupForienDID(signer)
			default:
				dc, err = c.SetupForienDIDQuorum(signer)
			}
			err := b.VerifySignature(dc)
			if err != nil {
				c.log.Error("Failed to verify signature", "err", err)
				return false
			}
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

func (c *Core) checkTokenIsPledged(wt string) bool {
	tokenType := token.RBTTokenType
	if c.testNet {
		tokenType = token.TestTokenType
	}
	b := c.W.GetLatestTokenBlock(wt, tokenType)
	if b == nil {
		c.log.Error("Invalid token chain block")
		return true
	}
	return c.checkIsPledged(b, wt)
}

//func (c *Core) checkTokenIsUnpledged(wt string) bool {
//	tokenType := token.RBTTokenType
//	if c.testNet {
//		tokenType = token.TestTokenType
//	}
//	b := c.W.GetLatestTokenBlock(wt, tokenType)
//	if b == nil {
//		c.log.Error("Invalid token chain block")
//		return true
//	}
//	return c.checkIsUnpledged(b, wt)
//}
//
//func (c *Core) getUnpledgeId(wt string) string {
//	tokenType := token.RBTTokenType
//	if c.testNet {
//		tokenType = token.TestTokenType
//	}
//	b := c.W.GetLatestTokenBlock(wt, tokenType)
//	if b == nil {
//		c.log.Error("Invalid token chain block")
//		return ""
//	}
//	return b.GetUnpledgeId()
//}
