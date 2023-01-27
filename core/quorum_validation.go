package core

import (
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/util"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) validateTokenOwnership(cr *ConensusRequest) bool {
	// ::TODO:: Need to implement
	for i := range cr.WholeTokens {
		c.log.Debug("Finding dht", "token", cr.WholeTokens[i])
		ids, err := c.GetDHTddrs(cr.WholeTokens[i])
		if err != nil || len(ids) == 0 {
			continue
		}
	}
	for i := range cr.WholeTokens {
		tcb := cr.WholeTCBlocks[i]
		h, s, err := wallet.GetTCHashSig(tcb)
		if err != nil {
			c.log.Error("Failed to get hash & signature", "err", err)
			return false
		}
		ch, err := wallet.TC2HashString(tcb)
		if err != nil {
			c.log.Error("Failed to calculate block hash", "err", err)
			return false
		}
		if h != ch {
			c.log.Error("Calculated block hash is not matching", "ch", ch, "rh", h)
			return false
		}
		var dc did.DIDCrypto
		switch wallet.GetTCTransType(tcb) {
		case wallet.TokenGeneratedType:
			if !c.testNet {
				c.log.Error("Invalid token on main net")
				return false
			}
			dc = c.SetupForienDID(cr.SenderDID)
		case wallet.TokenTransferredType:
			dc = c.SetupForienDID(wallet.GetTCSenderDID(tcb))
		// ::TODO:: need to add other verification
		default:
			return false
		}
		if !c.validateSignature(dc, h, s) {
			return false
		}
	}
	// for i := range cr.WholeTokens {
	// 	c.log.Debug("Requesting Token status")
	// 	ts := TokenPublish{
	// 		Token: cr.WholeTokens[i],
	// 	}
	// 	c.ps.Publish(TokenStatusTopic, &ts)
	// 	c.log.Debug("Finding dht", "token", cr.WholeTokens[i])
	// 	ids, err := c.GetDHTddrs(cr.WholeTokens[i])
	// 	if err != nil || len(ids) == 0 {
	// 		c.log.Error("Failed to find token owner", "err", err)
	// 		crep.Message = "Failed to find token owner"
	// 		return c.l.RenderJSON(req, &crep, http.StatusOK)
	// 	}
	// 	if len(ids) > 1 {
	// 		// ::TODO:: to more check to findout right pwner
	// 		c.log.Error("Mutiple owner found for the token", "token", cr.WholeTokens, "owners", ids)
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
