package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) PublishTokenChainDetailsEvent(tokenDetails []model.SendTokenDetailsInfo) {
	total := len(tokenDetails)
	if total == 0 {
		c.log.Info("Nothing to publish")
		return
	}

	// ------------------------------First publishing  TOKEN DETAILS Info ------------------------------
	for i := 0; i < total; i += defaultBatchSize {
		end := i + defaultBatchSize
		if end > total {
			end = total
		}

		batch := tokenDetails[i:end]
		event := model.TokenChainDetailsEvent{
			PublisherPeerID: c.peerID,
			TokenDetails:    batch,
			BatchNumber:     i/defaultBatchSize + 1,
		}

		data, _ := json.Marshal(event)

		envelope := PubSubEnvelope{
			Type: "token",
			Data: data,
		}

		payload, _ := json.Marshal(envelope)

		if err := c.ps.Publish("token_chain_details", payload); err != nil {
			c.log.Error("Failed to publish token batch", "err", err)
			continue
		}

		c.log.Info("Published token batch", "range", fmt.Sprintf("%d-%d", i, end-1))
		time.Sleep(publishDelay)
	}

	// ----------------------------Publishing  TXN HISTORY to the same pubsub------------------------------
	c.publishTransactionHistory()

	c.log.Info("All batches published")
}

func (c *Core) publishTransactionHistory() {
	txns, err := c.w.GetAllTransactionHistory()
	if err != nil {
		c.log.Error("Failed to read TransactionHistory", "err", err)
		return
	}

	fttxns, err := c.w.GetAllFTTransactionHistory()
	if err != nil {
		c.log.Error("Failed to read FTTransactionHistory", "err", err)
		return
	}

	var records []model.FullNodeTxnHistoryInfo

	for _, t := range txns {
		records = append(records, model.FullNodeTxnHistoryInfo{
			TransactionID:    t.TransactionID,
			TransactionValue: t.Amount,
			BlockHash:        t.BlockID,
		})
	}

	for _, t := range fttxns {
		records = append(records, model.FullNodeTxnHistoryInfo{
			TransactionID:    t.TransactionID,
			TransactionValue: t.Amount,
			BlockHash:        t.BlockID,
		})
	}

	chunkSize := 500
	for i := 0; i < len(records); i += chunkSize {
		end := i + chunkSize
		if end > len(records) {
			end = len(records)
		}

		batch := records[i:end]
		data, _ := json.Marshal(batch)

		env := PubSubEnvelope{
			Type: "txn",
			Data: data,
		}

		payload, _ := json.Marshal(env)

		if err := c.ps.Publish("token_chain_details", payload); err != nil {
			c.log.Error("Failed publishing txn history batch", "err", err)
		} else {
			c.log.Info("Published txn history batch", "size", len(batch))
		}

		time.Sleep(publishDelay)
	}
}
