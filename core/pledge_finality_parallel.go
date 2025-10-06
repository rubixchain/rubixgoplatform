package core

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// parallelQuorumPledgeFinality processes pledge finality updates in parallel
func (c *Core) parallelQuorumPledgeFinality(cr *ConensusRequest, newBlock *block.Block, newTokenStateHashes []string, transactionId string) error {
	c.log.Debug("Proceeding for pledge finality (parallel)")
	c.qlock.Lock()
	pd, ok1 := c.pd[cr.ReqID]
	cs, ok2 := c.quorumRequest[cr.ReqID]
	c.qlock.Unlock()
	if !ok1 || !ok2 {
		c.log.Error("Invalid pledge request")
		return fmt.Errorf("invalid pledge request")
	}

	// Prepare all update requests
	type updateJob struct {
		quorumDID     string
		pledgedTokens []string
		peer          *ipfsport.Peer
		qAddress      string
	}
	fmt.Println("Quorumlist length :", len(cr.QuorumList))
	fmt.Println("Quorum list :", cr.QuorumList)
	jobs := make([]updateJob, 0)
	for k, v := range pd.PledgedTokens {
		p, ok := cs.P[k]
		if !ok || p == nil {
			c.log.Error("Invalid pledge request for DID", "did", k)
			return fmt.Errorf("invalid pledge request")
		}

		var qAddress string
		for _, quorumValue := range cr.QuorumList {
			if strings.Contains(quorumValue, p.GetPeerDID()) {
				qAddress = quorumValue
				break
			}
		}

		jobs = append(jobs, updateJob{
			quorumDID:     k,
			pledgedTokens: v,
			qAddress:      qAddress,
		})
	}

	// Process updates in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, len(jobs))
	semaphore := make(chan struct{}, 3) // Limit concurrent connections

	startTime := time.Now()
	for _, job := range jobs {
		wg.Add(1)
		go func(j updateJob) {
			defer wg.Done()

			// Rate limiting
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			qPeer, err := c.getPeer(j.qAddress)
			if err != nil {
				c.log.Error("Quorum not connected", "err", err, "address", j.qAddress)
				errChan <- err
				return
			}
			defer qPeer.Close()

			var br model.BasicResponse
			ur := UpdatePledgeRequest{
				Mode:                        cr.Mode,
				PledgedTokens:               j.pledgedTokens,
				TokenChainBlock:             newBlock.GetBlock(),
				TransactionID:               transactionId,
				TransferredTokenStateHashes: newTokenStateHashes,
				TransactionEpoch:            cr.TransactionEpoch,
			}

			err = qPeer.SendJSONRequest("POST", APIUpdatePledgeToken, nil, &ur, &br, true)
			if err != nil {
				c.log.Error("Failed to update pledge token status", "err", err, "quorum", j.quorumDID)
				errChan <- fmt.Errorf("failed to update pledge token status for %s: %v", j.quorumDID, err)
				return
			}
			if !br.Status {
				c.log.Error("Failed to update pledge token status", "msg", br.Message, "quorum", j.quorumDID)
				errChan <- fmt.Errorf("failed to update pledge token status for %s: %s", j.quorumDID, br.Message)
				return
			}

			c.log.Debug("Pledge finality updated for quorum", "did", j.quorumDID, "tokens", len(j.pledgedTokens))
		}(job)
	}

	// Wait for all updates to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		c.log.Error("Pledge finality failed", "errors", len(errs), "total", len(jobs))
		return fmt.Errorf("pledge finality failed for %d/%d quorums", len(errs), len(jobs))
	}

	c.log.Info("Parallel pledge finality completed",
		"quorums", len(jobs),
		"duration", time.Since(startTime))

	return nil
}