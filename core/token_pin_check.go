package core

import (
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// Method checks for multiple Pins on token
// if there are multiple owners the list of owners is returned back
func (c *Core) pinCheck(token string, senderPeerId string, receiverPeerId string) (bool, []string, error) {

	var owners []string
	provList, err := c.GetDHTddrs(token)
	if err != nil {
		c.log.Error("Error triggered while fetching providers ", "error", err)
		return false, nil, err
	}

	if len(provList) == 0 {
		return false, provList, nil
	}

	if len(provList) == 1 {
		for _, peerId := range provList {
			if peerId != senderPeerId {
				c.log.Error("Sender peer not exist in provider list", "peerID", peerId)
				return true, provList, nil
			} else {
				return false, nil, nil
			}
		}
	}

	var knownPeer []string
	knownPeer = append(knownPeer, senderPeerId)
	knownPeer = append(knownPeer, receiverPeerId)

	if len(provList) >= 2 {
		owners = provList
		t := c.removeStrings(owners, knownPeer)
		if len(t) == 0 {
			c.log.Info("Pins help by current sender and receiver, pass")
			return false, nil, nil
		} else {
			peerIdRolemap := make(map[string]int)
			for _, peerId := range t {
				p, err := c.connectPeer(peerId)
				if err != nil {
					c.log.Error("Error connecting to peer ", "peerId", peerId, "err", err)
					return true, nil, err
				}
				req := PinStatusReq{
					Token: token,
				}
				var psr PinStatusRes
				err = p.SendJSONRequest("POST", APIDhtProviderCheck, nil, &req, &psr, true)
				if err != nil {
					c.log.Error("Failed to get response from Peer", "err", err)
					return false, nil, err
				}
				if psr.Status {
					peerIdRolemap[peerId] = psr.Role
				}
			}

			for peerId, _ := range peerIdRolemap {
				if peerIdRolemap[peerId] == wallet.OwnerRole {
					c.log.Error("Token has multiple Pins")
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
