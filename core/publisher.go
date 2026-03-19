package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const txnBatchSize = 500

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
		time.Sleep(delayInPublishingTCDetails)
	}

	c.log.Info("All batches of token details got published")
}

func (c *Core) PublishTransactionHistory() {
	c.log.Info("Starting publishTransactionHistory in batches...")

	var totalRBTCount, totalFTCount int
	var totalPublished int

	// -------------------- RBT Transaction History --------------------
	offset := 0
	for {
		txns, err := c.w.GetTransactionHistoryChunk(txnBatchSize, offset)
		if err != nil {
			if strings.Contains(err.Error(), "no records found") {
				c.log.Debug("No more RBT transaction records to read, breaking loop")
				break
			}
			c.log.Error("Failed to read RBT TransactionHistory chunk", "err", err)
			break
		}
		if len(txns) == 0 {
			break
		}

		offset += len(txns)
		totalRBTCount += len(txns)

		var records []model.FullNodeTxnHistoryInfo
		for _, t := range txns {
			records = append(records, model.FullNodeTxnHistoryInfo{
				TransactionID:    t.TransactionID,
				TransactionValue: t.Amount,
				BlockHash:        t.BlockID,
			})
		}

		c.publishTxnBatch(records)
		totalPublished += len(records)

		c.log.Info("Published RBT TransactionHistory batch",
			"batchSize", len(records),
			"offset", offset)
		time.Sleep(delayInPublshingTxnHistory)
	}

	// -------------------- FT Transaction History --------------------
	offset = 0
	for {
		fttxns, err := c.w.GetFTTransactionHistoryChunk(txnBatchSize, offset)
		if err != nil {
			c.log.Error("Failed to read FTTransactionHistory chunk", "err", err)
			break
		}
		if len(fttxns) == 0 {
			break
		}

		offset += len(fttxns)
		totalFTCount += len(fttxns)

		var records []model.FullNodeTxnHistoryInfo
		for _, t := range fttxns {
			records = append(records, model.FullNodeTxnHistoryInfo{
				TransactionID:    t.TransactionID,
				TransactionValue: t.Amount,
				BlockHash:        t.BlockID,
			})
		}

		c.publishTxnBatch(records)
		totalPublished += len(records)

		c.log.Info("Published FT TransactionHistory batch",
			"batchSize", len(records),
			"offset", offset)
		time.Sleep(delayInPublshingTxnHistory)
	}

	c.log.Info("✅ Transaction history publishing completed",
		"RBTCount", totalRBTCount,
		"FTCount", totalFTCount,
		"TotalPublished", totalPublished,
	)
}

func (c *Core) prepareTokenDetailsForRBT(tokens []models.Token) []model.SendTokenDetailsInfo {
	var result []model.SendTokenDetailsInfo
	for _, token := range tokens {
		tokenType := c.TokenType(RBTString)
		if token.TokenValue != float64(1) {
			tokenType = c.TokenType(PartString)
		}
		latestBlock := c.w.GetLatestTokenBlock(token.TokenID, tokenType)
		if latestBlock == nil {
			c.log.Error("Failed to get latest block for token", "token", token)
			continue
		}
		blockHeight, err := latestBlock.GetBlockNumber(token.TokenID)
		if err != nil {
			c.log.Error("Failed to get latest block height of token", "token", token, "error", err)
			continue
		}
		result = append(result, model.SendTokenDetailsInfo{
			Token:            token.TokenID,
			TokenChainLength: blockHeight,
			TokenType:        tokenType,
			Did:              token.DID,
			AssetType:        RBTTokenType,
			TokenValue:       token.TokenValue,
		})
	}
	return result
}

func (c *Core) prepareTokenDetailsForFT(tokens []models.Token) []model.SendTokenDetailsInfo {
	var result []model.SendTokenDetailsInfo
	for _, token := range tokens {
		tokenType := c.TokenType(FTString)
		latestBlock := c.w.GetLatestTokenBlock(token.TokenID, tokenType)
		if latestBlock == nil {
			c.log.Error("Failed to get latest block for token", "token", token)
			continue
		}
		blockHeight, err := latestBlock.GetBlockNumber(token.TokenID)
		if err != nil {
			c.log.Error("Failed to get latest block height of token", "token", token, "error", err)
			continue
		}
		result = append(result, model.SendTokenDetailsInfo{
			Token:            token.TokenID,
			TokenChainLength: blockHeight,
			TokenType:        tokenType,
			Did:              token.DID,
			AssetType:        FTTokenType,
		})
	}
	return result
}

func (c *Core) prepareTokenDetailsForNFT(tokens []wallet.NFT) []model.SendTokenDetailsInfo {
	var result []model.SendTokenDetailsInfo
	for _, token := range tokens {
		tokenType := c.TokenType(NFTString)
		latestBlock := c.w.GetLatestTokenBlock(token.TokenID, tokenType)
		if latestBlock == nil {
			c.log.Error("Failed to get latest block for token", "token", token)
			continue
		}
		blockHeight, err := latestBlock.GetBlockNumber(token.TokenID)
		if err != nil {
			c.log.Error("Failed to get latest block height of token", "token", token, "error", err)
			continue
		}
		result = append(result, model.SendTokenDetailsInfo{
			Token:            token.TokenID,
			TokenChainLength: blockHeight,
			TokenType:        tokenType,
			Did:              token.DID,
			AssetType:        NFTTokenType,
		})
	}
	return result
}

func (c *Core) prepareTokenDetailsForSC(tokens []models.Token) []model.SendTokenDetailsInfo {
	var result []model.SendTokenDetailsInfo
	for _, token := range tokens {
		tokenType := c.TokenType(SmartContractString)
		latestBlock := c.w.GetLatestTokenBlock(token.TokenID, tokenType)
		if latestBlock == nil {
			c.log.Error("Failed to get latest block for token", "token", token)
			continue
		}
		blockHeight, err := latestBlock.GetBlockNumber(token.TokenID)
		if err != nil {
			c.log.Error("Failed to get latest block height of token", "token", token, "error", err)
			continue
		}
		result = append(result, model.SendTokenDetailsInfo{
			Token:            token.TokenID,
			TokenChainLength: blockHeight,
			TokenType:        tokenType,
			Did:              token.DID,
			AssetType:        SmartContractTokenType,
		})
	}
	return result
}
func (c *Core) publishTxnBatch(records []model.FullNodeTxnHistoryInfo) {
	if len(records) == 0 {
		c.log.Info("There are no records to publish transaction history batch")
		return
	}

	data, err := json.Marshal(records)
	if err != nil {
		c.log.Error("Failed to marshal txn batch", "err", err)
		return
	}

	env := PubSubEnvelope{
		Type: "txn",
		Data: data,
	}

	payload, err := json.Marshal(env)
	if err != nil {
		c.log.Error("Failed to marshal PubSub envelope", "err", err)
		return
	}

	if err := c.ps.Publish("token_chain_details", payload); err != nil {
		c.log.Error("Failed publishing txn history batch", "err", err)
	} else {
		c.log.Debug("Published txn history batch successfully", "size", len(records))
	}
}
