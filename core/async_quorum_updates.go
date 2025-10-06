package core

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// AsyncQuorumUpdateManager handles background updates to previous quorums
type AsyncQuorumUpdateManager struct {
	core *Core
	log  logger.Logger
}

// PreviousQuorumUpdate represents an update to send to a previous quorum
type PreviousQuorumUpdate struct {
	PreviousQuorumDID     string
	PreviousQuorumPeerID  string
	TokenIDTokenStateHash string
}

// UpdatePreviousQuorumsAsync updates previous quorums about exhausted tokens in the background
func (c *Core) UpdatePreviousQuorumsAsync(updates []PreviousQuorumUpdate) {
	if !c.cfg.CfgData.TrustedNetwork {
		// In non-trusted networks, do synchronous updates
		c.updatePreviousQuorumsSynchronous(updates)
		return
	}

	// For trusted networks, update in background
	go func() {
		c.log.Info("Starting async previous quorum updates", "count", len(updates))
		startTime := time.Now()

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, 5) // Limit concurrent connections
		successCount := 0
		failCount := 0
		var mu sync.Mutex

		for _, update := range updates {
			wg.Add(1)
			go func(u PreviousQuorumUpdate) {
				defer wg.Done()

				// Rate limiting
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				previousQuorumAddress := u.PreviousQuorumPeerID + "." + u.PreviousQuorumDID
				previousQuorumPeer, err := c.getPeer(previousQuorumAddress)
				if err != nil {
					c.log.Debug("Failed to connect to previous quorum", 
						"address", previousQuorumAddress, 
						"error", err)
					mu.Lock()
					failCount++
					mu.Unlock()
					return
				}
				defer previousQuorumPeer.Close()

				updateTokenHashDetailsQuery := make(map[string]string)
				updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = u.TokenIDTokenStateHash
				
				err = previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, 
					updateTokenHashDetailsQuery, nil, nil, true)
				
				mu.Lock()
				if err != nil {
					failCount++
					c.log.Debug("Failed to update previous quorum", 
						"did", u.PreviousQuorumDID,
						"error", err)
				} else {
					successCount++
				}
				mu.Unlock()
			}(update)
		}

		wg.Wait()
		
		c.log.Info("Async previous quorum updates completed",
			"total", len(updates),
			"success", successCount,
			"failed", failCount,
			"duration", time.Since(startTime))
	}()
}

// updatePreviousQuorumsSynchronous is the original synchronous implementation
func (c *Core) updatePreviousQuorumsSynchronous(updates []PreviousQuorumUpdate) {
	for _, update := range updates {
		previousQuorumAddress := update.PreviousQuorumPeerID + "." + update.PreviousQuorumDID
		previousQuorumPeer, err := c.getPeer(previousQuorumAddress)
		if err != nil {
			c.log.Error("Unable to retrieve peer information", 
				"peerID", update.PreviousQuorumPeerID, 
				"error", err)
			continue
		}

		updateTokenHashDetailsQuery := make(map[string]string)
		updateTokenHashDetailsQuery["tokenIDTokenStateHash"] = update.TokenIDTokenStateHash
		previousQuorumPeer.SendJSONRequest("POST", APIUpdateTokenHashDetails, 
			updateTokenHashDetailsQuery, nil, nil, true)
		previousQuorumPeer.Close()
	}
}

// BuildPreviousQuorumUpdates prepares the list of updates for previous quorums
func (c *Core) BuildPreviousQuorumUpdates(ti []contract.TokenInfo, sc *contract.Contract) ([]PreviousQuorumUpdate, error) {
	var updates []PreviousQuorumUpdate
	peerInfoCache := make(map[string]*wallet.DIDPeerMap)

	for _, tokeninfo := range ti {
		b := c.w.GetLatestTokenBlock(tokeninfo.Token, tokeninfo.TokenType)

		blockHeight, err := b.GetBlockNumber(tokeninfo.Token)
		if err != nil {
			c.log.Error("failed to get latest block height of token", "token", tokeninfo.Token)
			continue
		}

		// Skip genesis blocks
		if blockHeight == 0 && tokeninfo.TokenValue == 1.0 {
			continue
		}

		previousQuorumDIDs, err := b.GetSigner()
		if err != nil {
			return nil, fmt.Errorf("unable to fetch previous quorum's DIDs for token: %v, err: %v", tokeninfo.Token, err)
		}

		// Skip if signer is sender
		if previousQuorumDIDs[0] == sc.GetSenderDID() {
			continue
		}

		// Get block ID and create hash
		bid, err := b.GetBlockID(tokeninfo.Token)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch current block id for Token %v, err: %v", tokeninfo.Token, err)
		}

		prevtokenIDTokenStateData := tokeninfo.Token + bid
		prevtokenIDTokenStateBuffer := bytes.NewBuffer([]byte(prevtokenIDTokenStateData))
		prevtokenIDTokenStateHash, err := IpfsAddWithBackoff(c.ipfs, prevtokenIDTokenStateBuffer, 
			ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
		if err != nil {
			return nil, fmt.Errorf("unable to get previous token state hash for token: %v, err: %v", tokeninfo.Token, err)
		}

		// Create updates for each previous quorum
		for _, previousQuorumDID := range previousQuorumDIDs {
			var previousQuorumInfo *wallet.DIDPeerMap
			var ok bool
			
			if previousQuorumInfo, ok = peerInfoCache[previousQuorumDID]; !ok {
				previousQuorumInfo, err = c.GetPeerDIDInfo(previousQuorumDID)
				if previousQuorumInfo.PeerID == "" || err != nil {
					c.log.Error("Unable to get peerID for signer DID", "did", previousQuorumDID)
					continue
				}
				peerInfoCache[previousQuorumDID] = previousQuorumInfo
			}

			updates = append(updates, PreviousQuorumUpdate{
				PreviousQuorumDID:     previousQuorumDID,
				PreviousQuorumPeerID:  previousQuorumInfo.PeerID,
				TokenIDTokenStateHash: prevtokenIDTokenStateHash,
			})
		}
	}

	return updates, nil
}