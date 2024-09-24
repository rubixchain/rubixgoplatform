package core

import (
	"fmt"
	"sync"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

type MultiPinCheckRes struct {
	Token  string
	Status bool
	Owners []string
	Error  error
}

func (c *Core) removeStrings(strings []string, targets []string, target string) []string {
	targetMap := make(map[string]bool)
	if targets == nil {
		targets = []string{target}
	}
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

	provList, err := c.GetDHTddrs(token)

	fmt.Println("prov list for token t : ", token)
	fmt.Println("provList : ", provList)
	if err != nil {
		c.log.Error("Error triggered while fetching providers ", "error", err)
		return
	}

	var ownersProv []string

	result.Token = token
	result.Status = false
	result.Owners = nil
	result.Error = nil

	switch len(provList) {
	case 0:
		c.log.Error(fmt.Sprintf("there are no providers for token : %v", token))
		result.Status = true
	case 1:
		if provList[0] != senderPeerId {
			c.log.Error(fmt.Sprintf("sender peer does not exist in provider list : %v", provList[0]))
			result.Status = true
			result.Owners = provList
		}
	default:
		provList = c.removeStrings(provList, nil, senderPeerId)
		if receiverPeerId != "" {
			provList = c.removeStrings(provList, nil, receiverPeerId)
		}
		if len(provList) != 0 {
			for _, peerId := range provList {
				p, err := c.connectPeer(peerId)
				if err != nil || p == nil {
					c.log.Error("Error connecting to peer ", "peerId", peerId, "err", err)
					continue
				}
				req := PinStatusReq{
					Token: token,
				}
				var psr PinStatusRes
				err = p.SendJSONRequest("POST", APIDhtProviderCheck, nil, &req, &psr, true)
				if err != nil {
					c.log.Error("Failed to get response from Peer", "err", err)
					continue
				}
				if psr.Role == wallet.OwnerRole || psr.Role == wallet.ParentTokenLockRole {
					ownersProv = append(ownersProv, peerId)
					continue
				}
			}
			if len(ownersProv) > 0 {
				result.Status = true
				result.Error = fmt.Errorf("token %v has multiple pins by %v", token, ownersProv)
			}
			result.Owners = ownersProv
		}
	}
	results[index] = result
}
