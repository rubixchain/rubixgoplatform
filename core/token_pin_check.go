package core

import (
	"sync"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

type MultiPinCheckRes struct {
	Token  string
	Status bool
	Owners []string
	Error  error
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

// Method checks for multiple Pins on token
// if there are multiple owners the list of owners is returned back
func (c *Core) pinCheck(token string, index int, senderPeerId string, receiverPeerId string, results []MultiPinCheckRes, wg *sync.WaitGroup) {

	defer wg.Done()
	var result MultiPinCheckRes
	result.Token = token
	var owners []string
	provList, err := c.GetDHTddrs(token)
	if err != nil {
		c.log.Error("Error triggered while fetching providers ", "error", err)
		result.Status = false
		result.Owners = nil
		result.Error = err
		results[index] = result
	}

	if len(provList) == 0 {
		result.Status = false
		result.Owners = provList
		result.Error = nil
		results[index] = result
	}

	if len(provList) == 1 {
		for _, peerId := range provList {
			if peerId != senderPeerId {
				c.log.Error("Sender peer not exist in provider list", "peerID", peerId)
				result.Status = true
				result.Owners = provList
				result.Error = nil
				results[index] = result
			} else {
				result.Status = false
				result.Owners = nil
				result.Error = nil
				results[index] = result
			}
		}
	}

	var knownPeer []string
	knownPeer = append(knownPeer, senderPeerId)
	if receiverPeerId != "" {
		knownPeer = append(knownPeer, receiverPeerId)
	}

	if len(provList) >= 2 {
		owners = provList
		t := c.removeStrings(owners, knownPeer)
		if len(t) == 0 {
			c.log.Info("Pins help by current sender and receiver, pass")
			result.Status = false
			result.Owners = nil
			result.Error = nil
			results[index] = result
		} else {
			peerIdRolemap := make(map[string]int)
			for _, peerId := range t {
				p, err := c.connectPeer(peerId)
				if err != nil || p == nil {
					c.log.Error("Error connecting to peer ", "peerId", peerId, "err", err)
					result.Status = true
					result.Owners = nil
					result.Error = err
					results[index] = result
					continue
				}
				req := PinStatusReq{
					Token: token,
				}
				var psr PinStatusRes
				err = p.SendJSONRequest("POST", APIDhtProviderCheck, nil, &req, &psr, true)
				if err != nil {
					c.log.Error("Failed to get response from Peer", "err", err)
					result.Status = false
					result.Owners = nil
					result.Error = err
					results[index] = result
				}
				if psr.Status {
					peerIdRolemap[peerId] = psr.Role
				}
			}

			for peerId, _ := range peerIdRolemap {
				if peerIdRolemap[peerId] == wallet.OwnerRole {
					c.log.Error("Token has multiple Pins")
					result.Status = true
					result.Owners = provList
					result.Error = nil
					results[index] = result
				}
			}
		}
	}
	c.log.Debug("Token does not have multiple pins")
	result.Status = false
	result.Owners = nil
	result.Error = nil
	results[index] = result
}
