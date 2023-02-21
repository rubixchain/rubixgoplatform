package core

import (
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (c *Core) validateTokenOwnership(cr *ConensusRequest, sc *contract.Contract) bool {
	// ::TODO:: Need to implement
	wt := sc.GetWholeTokens()
	wtdID := sc.GetWholeTokensID()
	for i := range wt {
		c.log.Debug("Finding dht", "token", wt[i])
		ids, err := c.GetDHTddrs(wt[i])
		if err != nil || len(ids) == 0 {
			continue
		}
	}
	address := cr.SenderPeerID + "." + sc.GetSenderDID()
	for i := range wt {
		err := c.syncTokenChainFrom(address, wtdID[i], wt[i])
		if err != nil {
			c.log.Error("Failed to sync token chain block", "err", err)
			return false
		}
		b := c.w.GetLatestTokenBlock(wt[i])
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
			h, s, err := b.GetHashSig(signer)
			if err != nil {
				c.log.Error("Failed to get hash & signature", "err", err)
				return false
			}
			var dc did.DIDCrypto
			switch b.GetTransType() {
			case block.TokenGeneratedType:
				dc, err = c.SetupForienDID(signer)
			default:
				dc, err = c.SetupForienDIDQuorum(signer)
			}

			if err != nil {
				c.log.Error("Failed to get did", "err", err)
				return false
			}
			if !c.validateSignature(dc, h, s) {
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
	c.log.Info("Received token hash", "hash", h)
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
	b := c.w.GetLatestTokenBlock(wt)
	if b == nil {
		c.log.Error("Invalid token chain block")
		return true
	}
	return c.checkIsPledged(b, wt)
}
