package core

import (
	"github.com/EnsurityTechnologies/uuid"
	"github.com/rubixchain/rubixgoplatform/core/did"
	"github.com/rubixchain/rubixgoplatform/core/model"
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

func (c *Core) multiplePincheck(tokenHash string, wtcBlock map[string]interface{}, cr *ConensusRequest) (bool, []string, error) {
	//tokenHash := cr.WholeTokens[i]
	c.log.Debug("Finding dht", "token", tokenHash)
	pinIds, err := c.GetDHTddrs(tokenHash)
	if err != nil {
		c.log.Error("Error in finding pins of token", "err", err)
		return true, nil, err
	}
	if len(pinIds) == 0 {
		c.log.Info("No pins found for token ", tokenHash)
		return false, nil, nil
	}
	if len(pinIds) >= 2 {

		//wtcBlock := cr.WholeTCBlocks[i]

		prevSenderDid := wallet.GetTCSenderDID(wtcBlock)

		var prevSenderDidList []string
		prevSenderDidList = append(prevSenderDidList, prevSenderDid)

		prevSenderPeerId, err := c.getPeerIds(prevSenderDidList)
		if err != nil {
			return true, nil, err
		}
		retain := prevSenderPeerId

		prevSenderPeerId = append(prevSenderPeerId, cr.SenderPeerID)
		prevSenderPeerId = append(prevSenderPeerId, cr.ReceiverPeerID)

		uniqueElements := make(map[string]bool)

		for _, element := range pinIds {
			uniqueElements[element] = true
		}

		for _, element := range prevSenderPeerId {
			if !uniqueElements[element] {
				uniqueElements[element] = true
			}
		}

		var resultList []string
		for element := range uniqueElements {
			resultList = append(resultList, element)
		}

		if len(resultList) > 0 {
			c.log.Error("Multiple Pins Found for token", tokenHash)
			c.log.Error("Owners are ", pinIds)
			return true, retain, nil
		}
	}
	return false, nil, nil
}

func (c *Core) getPeerIds(didList []string) ([]string, error) {

	sd, _ := c.sd[ExplorerService]
	var peerIdList []string
	exp := model.ExploreModel{
		Cmd:     ExpDIDPeerMapCmd,
		PeerID:  c.peerID,
		DIDList: didList,
	}
	err := c.PublishExplorer(&exp)
	if err != nil {
		c.log.Error("Failed to publish message to explorer", "err", err)
		return nil, err
	}

	var didPeerIdMap ExplorerNodeDIDMap

	for _, did := range exp.DIDList {
		err := sd.db.FindNew(uuid.Nil, NodeDIDMapTable, "DID=?", &didPeerIdMap, did)
		if err != nil {
			c.log.Error("Failed to create did map table", "err", err)
			return nil, err
		}
		peerIdList = append(peerIdList, didPeerIdMap.PeerID)
	}
	return peerIdList, nil
}
