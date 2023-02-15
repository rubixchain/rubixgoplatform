package core

import "strings"

var RoleMap = map[int]string{
	1: "owner",
	2: "quorum",
	3: "prevSender",
	4: "receiver",
	5: "parentTokenLock",
	6: "did",
	7: "staking",
	8: "pledging",
}

// Method checks for multiple Pins on token
// if there are multiple owners the list of owners is returned back
func (c *Core) pinCheck(token string, cr *ConensusRequest) (bool, []string, error) {
	c.log.Debug("Finding the list of Providers for Token", token)

	var owners []string
	provList, err := c.GetDHTddrs(token)
	if err != nil {
		c.log.Error("Error triggered while fetching providers ", "error", err)
		return false, nil, err
	}

	c.log.Debug("Providers for the Token", token)
	c.log.Debug("are ", provList)

	if len(provList) == 0 {
		c.log.Info("No pins found for Token", token)
		return false, provList, nil
	}

	if len(provList) == 1 {
		for _, peerId := range provList {
			if peerId != cr.SenderPeerID {
				c.log.Debug("Pin is not held by current Sender for token", token)
				c.log.Debug("peer Id that holds the pin", peerId)
				return true, provList, nil
			} else {
				c.log.Debug("Pin is held by current Sender for Token", token)
				return false, nil, nil
			}
		}
	}

	var knownPeer []string
	knownPeer = append(knownPeer, cr.SenderPeerID)
	knownPeer = append(knownPeer, cr.ReceiverPeerID)

	if len(provList) >= 2 {
		owners = provList
		t := c.removeStrings(owners, knownPeer)
		c.log.Debug("List after removing known peers", t)
		if len(t) == 0 {
			c.log.Info("Pins help by current sender and receiver, pass")
			return false, nil, nil
		} else {
			var peerIdRolemap map[string]string
			for _, peerId := range t {
				p, err := c.connectPeer(peerId)
				if err != nil {
					c.log.Error("Error connecting to peer ", "peerId", peerId)
					c.log.Error("", "error", err)
					return true, nil, err
				}
				c.log.Debug("Peer connection established", "PeerID", peerId)
				req := PinStatusReq{
					Token: token,
				}
				var psr PinStatusRes
				err1 := p.SendJSONRequest("POST", APIDhtProviderCheck, nil, &req, &psr, true)
				if err != nil {
					c.log.Error("Failed to get response from Peer", "error", err1)
					return false, nil, err1
				}

				if psr.DID == "" && psr.Token == "" && psr.Role == 0 && psr.FuncID == 0 {
					c.log.Debug("PeerId", peerId, "Does not have any info on the token", token)
					c.log.Debug("Allowing pass of multi pin check")
				} else {
					peerIdRolemap[peerId] = c.roleIDtoStr(psr.Role)
				}

			}

			for peerId, _ := range peerIdRolemap {
				if strings.Compare(peerIdRolemap[peerId], "owner") == 0 {
					c.log.Debug("Token has multiple Pins")
					return true, provList, nil
				}
			}
		}
	}
	c.log.Debug("Token does not have multiple pins")
	return false, nil, nil
}

func (c *Core) removeStrings(strings []string, targets []string) []string {
	targetMap := make(map[string]bool)
	for _, t := range targets {
		targetMap[t] = true
	}

	result := []string{}
	for _, s := range strings {
		if !targetMap[s] {
			result = append(result, s)
		}
	}

	return result
}

func (c *Core) roleIDtoStr(roleId int) string {
	return RoleMap[roleId]
}
