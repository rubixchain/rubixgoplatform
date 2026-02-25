package core

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Initialize the transaction processor
	c.initDynamicTxnProcessor()

	topic := RubixTxnTopic
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		c.log.Error("Unable to subscribe to topic", "topic", topic, "error", err)
		return
	}
	c.log.Info("Successfully subscribed to topic: " + topic)
}

// Enhanced callback with dynamic scaling integration
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
	var newEvent model.PubSubTxnInfo
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to parse published event", "error", err, "data", string(data))
		return
	}

	// add publisher to peer did table
	unknownDIDType := -1
	publisherDetails := &wallet.DIDPeerMap{
		DID:     newEvent.PublisherDID,
		PeerID:  peerID,
		DIDType: &unknownDIDType,
	}
	err = c.AddPeerDetails(*publisherDetails)
	if err != nil {
		c.log.Error("failed to add publisher info to DB")
	}

	// Check for duplicate transactions
	if _, exists := c.txnProcessor.processedTxns.LoadOrStore(newEvent.BlockHash, time.Now()); exists {
		c.log.Info("Duplicate transaction ignored", "blockHash", newEvent.BlockHash)
		return
	}

	// INCREMENT COUNTER when new transaction is processed
	atomic.AddInt64(&c.txnProcessor.processedTxnCount, 1)

	c.log.Info("Received transaction", "blockHash", newEvent.BlockHash, "mode", newEvent.AssetType)

	// Update queue length metric for dynamic scaling
	currentQueueLen := int64(len(c.txnProcessor.txnQueue))
	c.txnProcessor.queueLength = currentQueueLen

	// Queue transaction for processing with enhanced timeout handling
	select {
	case c.txnProcessor.txnQueue <- &newEvent:
		c.log.Debug("Transaction queued successfully",
			"blockHash", newEvent.BlockHash,
			"queueLength", currentQueueLen)

	case <-time.After(5 * time.Second):
		c.log.Error("Failed to queue transaction - queue full",
			"blockHash", newEvent.BlockHash,
			"queueLength", len(c.txnProcessor.txnQueue))

		if currentQueueLen > int64(c.txnProcessor.queueThreshold) {
			c.log.Warn("Queue threshold exceeded - scaling may be needed",
				"current", currentQueueLen,
				"threshold", c.txnProcessor.queueThreshold)
		}
		return

	case <-c.txnProcessor.ctx.Done():
		c.log.Info("Transaction processor shutting down")
		return
	}
}

// Process transaction with retry mechanism
func (c *Core) processTxnWithRetry(txnEvent *model.PubSubTxnInfo, workerID int) {
	var lastErr error

	for attempt := 0; attempt < c.txnProcessor.maxRetries; attempt++ {
		if attempt > 0 {
			c.log.Info("Retrying transaction processing",
				"blockHash", txnEvent.BlockHash,
				"attempt", attempt+1,
				"workerID", workerID)
			time.Sleep(c.txnProcessor.retryDelay * time.Duration(attempt))
		}

		err := c.processSingleTransaction(txnEvent)
		if err == nil {
			c.log.Info("Transaction processed successfully",
				"blockHash", txnEvent.BlockHash,
				"workerID", workerID)
			return
		}

		lastErr = err
		c.log.Error("Transaction processing failed",
			"blockHash", txnEvent.BlockHash,
			"attempt", attempt+1,
			"error", err,
			"workerID", workerID)
	}

	// All retries exhausted - handle failure
	c.handleFailedTransaction(txnEvent, lastErr)
}

// Enhanced single transaction processing with better error handling
func (c *Core) processSingleTransaction(newEvent *model.PubSubTxnInfo) error {
	receiverDid := newEvent.ReceiverDID

	// Initialize block with error handling
	txnBlock := block.InitBlock(newEvent.TxnBlock, nil)
	if txnBlock == nil {
		return fmt.Errorf("failed to initialize transaction block for txn %s", newEvent.BlockHash)
	}
	txnBlockType := newEvent.BlockType
	if txnBlockType == block.TokenTransferredType || txnBlockType == block.TokenDeployedType || txnBlockType == block.TokenExecutedType {
		TransactionIDFromTheTransactionBlock := newEvent.TransactionID

		transaction := model.FullNodeTxnHistoryInfo{
			TransactionID:    TransactionIDFromTheTransactionBlock,
			TransactionValue: newEvent.TransactionValue,
			BlockHash:        newEvent.BlockHash,
		}
		transactionHistErr := c.w.AddTransactionsToFullNodeTransactionHistoryTable(&transaction)
		if transactionHistErr != nil {
			c.log.Error("faile to add transaction details to Fullnode transaction history table for the transaction: ", transaction.TransactionID, "error", transactionHistErr)
		}

	}

	currentOwner := txnBlock.GetOwner()
	tokensList := txnBlock.GetTransTokens()

	if len(tokensList) == 0 {
		return fmt.Errorf("no tokens found in transaction %s", newEvent.BlockHash)
	}

	switch newEvent.AssetType {
	case RBTTokenType, FTTokenType:
		return c.processTransferTransaction(newEvent, txnBlock, tokensList, receiverDid, currentOwner)

	case SmartContractTokenType, NFTTokenType:
		return c.processContractTransaction(newEvent, txnBlock, tokensList[0], currentOwner)

	default:
		return fmt.Errorf("unsupported transaction mode: %v", newEvent.AssetType)
	}
}

// Separate transfer transaction processing
func (c *Core) processTransferTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokensList []string, receiverDid, currentOwner string) error {
	var errors []string

	for _, tokenId := range tokensList {
		if err := c.processTransferToken(newEvent, txnBlock, tokenId, receiverDid, currentOwner); err != nil {
			errors = append(errors, fmt.Sprintf("token %s: %v", tokenId, err))
			continue
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("transfer processing errors: %v", errors)
	}

	return nil
}

// Process individual transfer token
func (c *Core) processTransferToken(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, receiverDid, currentOwner string) error {
	// TokenGenuinity check function
	err := c.GetTokenContentAndValidate(tokenId, newEvent.AssetType)
	if err != nil {
		wrappedErr := fmt.Errorf(
			"failed to validate token content, token %s: %w",
			tokenId,
			err,
		)
		c.log.Error(wrappedErr.Error())
		return wrappedErr
	}

	// Token generated type handling
	if newEvent.BlockType == block.TokenGeneratedType {
		if currentOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch for token generation: expected %s, got %s", currentOwner, newEvent.PublisherDID)
		}
		//Todo: Add a signature check here
		// valid, err := c.validateSigner(blk, "", p)
		// if !valid || err != nil {
		// 	c.log.Error("Failed to validate token signer for token", "token", tokenSyncInfo.TokenID, "err", err)
		// 	return fmt.Errorf("failed to validate token signer for %s", tokenSyncInfo.TokenID)
		// }
		//check that blockNumber which we are going to add is genesis block
		txnBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			return fmt.Errorf("failed to get the transaction block number for the token%s, err: %v", tokenId, err)
		}
		if txnBlockNumber != 0 {
			return fmt.Errorf("Incoming transaction block number is not zero, in case of genesis block: %v", err)
		}
		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add generated block to token chain, err: %v", err)
		}

		// update block height if required
		latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			c.log.Error("failed to get block height")
		}
		newEvent.LatestBlockHeight = latestBlockHeight
		syncStatus := wallet.SyncCompleted
		receivedBlocks := ReceivedBlock{
			GenesisBlock: txnBlock,
			LatestBlock:  txnBlock,
		}
		return c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlocks, newEvent, syncStatus)
	}

	// Regular transfer processing with enhanced validation
	return c.processRegularTransfer(newEvent, txnBlock, tokenId, receiverDid)
}

// Process regular transfer with enhanced validation
func (c *Core) processRegularTransfer(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, receiverDid string) error {
	// Get current and latest block numbers
	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number: %v", err)
	}
	// currentBlockHash, err := txnBlock.GetHash()
	// if err != nil {
	// 	return fmt.Errorf("failed to get current block hash: %v", err)
	// }
	// txnBlockType := txnBlock.GetTransType()
	tokenType := txnBlock.GetTokenType(tokenId)
	txnBlockOwner := txnBlock.GetOwner()
	txnBlockID, err := txnBlock.GetBlockID(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get the transaction blockID for the token%s", tokenId)
	}

	var p *ipfsport.Peer
	if currentBlockNumber != 0 {
		latestTokenBlock := c.w.GetFullNodeLatestTokenBlock(tokenId, tokenType)
		if latestTokenBlock == nil {
			//connect to publisher and fetch complete token chain
			p, err = c.getPeer(newEvent.PublisherDID)
			if err != nil {
				c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
				return fmt.Errorf("failed to open peer connection with publisher %s", newEvent.PublisherDID)
			}
			defer p.Close()
			tokenSyncInfo := &TokenSyncInfo{
				TokenID:   tokenId,
				TokenType: tokenType,
				AssetType: newEvent.AssetType,
			}
			err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
			if err != nil {
				handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, "", txnBlockOwner, fmt.Sprintf("tokenchian is wrong for the token%s is wrong,error %v", tokenId, err))
				if handled {
					if handleErr != nil {
						return handleErr
					}
					return nil
				}
				return fmt.Errorf("failed to sync the token chain for the token%s", tokenId)
			}
			c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
			return nil
		}
		previousOwner := latestTokenBlock.GetOwner()
		// LatestBlockHash, err := latestTokenBlock.GetHash()
		// if err != nil {
		// 	return fmt.Errorf("failed to get the latest block hash: %v", err)
		// }

		latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
		if err != nil {
			return fmt.Errorf("failed to get latest block number: %v", err)
		}

		// Check for missing blocks
		if latestBlockNumber+1 != currentBlockNumber {
			if latestBlockNumber == currentBlockNumber {
				latestBlockHash, _ := latestTokenBlock.GetHash()
				if newEvent.BlockHash == latestBlockHash {
					return nil
				}
				//add this token to double spend tokens table
				doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
					TokenID:        tokenId,
					AssetType:      newEvent.AssetType,
					TokenType:      tokenType,
					PublisherDID:   newEvent.PublisherDID,
					ClaimedOwnerI:  previousOwner,
					ClaimedOwnerII: txnBlockOwner,
					ErrorMessage:   fmt.Sprintf("block with the same block number exist, dual ownership issue or same did would have double spent the token, tokenID:%s", tokenId),
				}
				err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
				if err != nil {
					errMsg := doubleSpentTokenInfo.ErrorMessage + "failed to update double spent token in tables"
					return fmt.Errorf("%v", errMsg)
				}

				return fmt.Errorf("block with the same block number exist, dual ownership issue or same did would have double spent the token, tokenID:%s", tokenId)
			} else if latestBlockNumber < currentBlockNumber {
				//connect to publisher and fetch complete token chain
				p, err := c.getPeer(newEvent.PublisherDID)
				if err != nil {
					c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
					return fmt.Errorf("failed to open peer connection with publisher %s", newEvent.PublisherDID)
				}
				defer p.Close()
				tokenSyncInfo := &TokenSyncInfo{
					TokenID:   tokenId,
					TokenType: tokenType,
					AssetType: newEvent.AssetType,
				}
				err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
				if err != nil {
					handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, previousOwner, txnBlockOwner, fmt.Sprintf("%s, %s both dids are claiming the same token or same did would have double spent the token,dual ownership issue", previousOwner, txnBlockOwner))
					if handled {
						if handleErr != nil {
							return handleErr
						}
						return nil
					}
					c.log.Error("Failed to sync chain", "token", tokenId, "err", err)
					info := &model.FailedToSyncTokenDetailsInfo{
						TokenID:   tokenId,
						TokenType: tokenType,
						AssetType: newEvent.AssetType,
						Did:       newEvent.PublisherDID,
						Reason:    fmt.Sprintf("failed to sync chain,err: %v", err),
					}

					if err := c.w.AddFailedTokensToTable(info); err != nil {
						c.log.Error("Failed to record failed token sync in DB", "token", tokenId, "error", err)
						return fmt.Errorf("failed to add token to Failed to sync tokens table: %v", err)
					} else {
						c.log.Info("Recorded failed token sync in DB", "token", tokenId)
						return nil
					}
				}
				c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
				return nil
				// return fmt.Errorf("mismatch of blocks detected: latest block number=%d, current block number=%d", latestBlockNumber, currentBlockNumber)
			} else {
				incomingBlockID := txnBlockID
				publishersLatestBlockNumber := currentBlockNumber
				//get Fullnode block whose block number is publisher side latest blockID
				fullnodesideBlockBytes, err := c.w.GetFullNodeTokenBlockByNumber(tokenId, tokenType, publishersLatestBlockNumber)
				fullnodesideBlock := block.InitBlock(fullnodesideBlockBytes, nil)
				fullnodesideBlockID, err := fullnodesideBlock.GetBlockID(tokenId)
				if err != nil {
					c.log.Error("failed to get blockID of the fullnode side block", "error", err)
					return fmt.Errorf("failed to get blockID of the fullnode side block,error%v", err)
				}
				fullnodesideBlockOwner := fullnodesideBlock.GetOwner()
				if incomingBlockID != fullnodesideBlockID {
					//add token to double spend tokens table, with an error saying
					//that both DID1 or DID2 are claimming the same token.
					doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
						TokenID:        tokenId,
						AssetType:      newEvent.AssetType,
						TokenType:      tokenType,
						PublisherDID:   newEvent.PublisherDID,
						ClaimedOwnerI:  fullnodesideBlockOwner,
						ClaimedOwnerII: txnBlockOwner,
						ErrorMessage:   fmt.Sprintf("%s, %s both dids are claiming the same token,dual ownership issue", fullnodesideBlockOwner, txnBlockOwner),
					}
					// store double spent token info in DoubleSpentTokens table, and remove it from respective tokens table
					err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
					if err != nil {
						errMsg := fmt.Sprintf("failed to update double spent token : %v, err: %v", tokenId, err)
						c.log.Error(errMsg)
						return fmt.Errorf("failed to update double spent token : %v, err: %v", tokenId, err)
					}
					c.log.Error("token has been added to double spend tokens table, tokenID", tokenId)
					return nil

				} else {
					c.log.Info("current transaction blk %salready exist with fullnode", txnBlockID)
					return nil
				}

			}
		}

		receivedBlock := ReceivedBlock{
			LatestBlock: txnBlock,
		}

		genesisBlock := c.w.GetFullNodeGenesisTokenBlock(tokenId, tokenType)
		if genesisBlock != nil {
			receivedBlock.GenesisBlock = genesisBlock
		}

		if currentBlockNumber == 0 {
			receivedBlock.GenesisBlock = txnBlock
		}
		// Do all 3 checks, If any check fails, handle the failure here also just before adding the block to leveldb.
		err = c.ValidateIncomingTokenBlock(*txnBlock, latestTokenBlock, tokenId, p, newEvent.AssetType)
		if err != nil {
			handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, previousOwner, txnBlockOwner, fmt.Sprintf("Incoming block %s is wrong,error %v", txnBlockID, err))
			if handled {
				if handleErr != nil {
					return handleErr
				}
				return nil
			}
		}

		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add block to token chain: %v", err)
		}
		// update block height if required
		latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			c.log.Error("failed to get block height")
		}
		newEvent.LatestBlockHeight = latestBlockHeight
		syncStatus := wallet.SyncCompleted
		// Add to database and blockchain

		if err := c.AddTokenToRespectiveTable(tokenId, receiverDid, receivedBlock, newEvent, syncStatus); err != nil {
			return fmt.Errorf("failed to add token to table: %v", err)
		}

		c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
		return nil
	}
	return nil
}

// Process contract-related transactions (Smart Contract and NFT operations)
func (c *Core) processContractTransaction(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId, currentOwner string) error {
	// Handle token generated type (new deployments)
	if newEvent.BlockType == block.TokenDeployedType {
		if currentOwner != newEvent.PublisherDID {
			return fmt.Errorf("publisher DID mismatch for contract deployment: expected %s, got %s", currentOwner, newEvent.PublisherDID)
		}
		//TODO: Add signature check here after taking the pull of vaishnav's changes from the development

		//check that blockNumber which we are going to add is zero for the genesis block		
		txnBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			return fmt.Errorf("failed to get the transaction block number for the token%s, err: %v", tokenId, err)
		}
		if txnBlockNumber != 0 {
			return fmt.Errorf("Incoming transaction block number is not zero, in case of genesis block: %v", err)
		}
		// Add block directly to token chain for new deployments
		if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
			return fmt.Errorf("failed to add contract block to token chain: %v", err)
		}

		// if it is a genesis block, then fetch token's ipfs content and store in psql db
		if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
			return fmt.Errorf("failed to add token's ipfs content to psql db, err: %v", err)
		}
		// update block height if required
		latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
		if err != nil {
			c.log.Error("failed to get block height")
		}
		newEvent.LatestBlockHeight = latestBlockHeight
		syncStatus := wallet.SyncCompleted
		receivedBlock := ReceivedBlock{
			LatestBlock:  txnBlock,
			GenesisBlock: txnBlock,
		}
		if err := c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlock, newEvent, syncStatus); err != nil {
			return fmt.Errorf("failed to add contract token to table: %v", err)
		}
		c.log.Info("New contract deployment processed", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
		return nil
	}

	// Handle existing contract executions with validation
	return c.processContractExecution(newEvent, txnBlock, tokenId)
}

// Process execution of existing contracts with block validation
func (c *Core) processContractExecution(newEvent *model.PubSubTxnInfo, txnBlock *block.Block, tokenId string) error {
	// Get token type and latest block
	tokenType := txnBlock.GetTokenType(tokenId)
	latestTokenBlock := c.w.GetFullNodeLatestTokenBlock(tokenId, tokenType)
	txnBlockOwner := txnBlock.GetOwner()
	txnBlockID, err := txnBlock.GetBlockID(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get the transaction blockID for the token%s", tokenId)
	}
	if latestTokenBlock == nil {
		//connect to publisher and fetch complete token chain
		p, err := c.getPeer(newEvent.PublisherDID)
		if err != nil {
			c.log.Error("failed to sync full sc chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
			return fmt.Errorf("failed to open peer connection with publisher - %v ", newEvent.PublisherDID)
		}
		defer p.Close()
		tokenSyncInfo := &TokenSyncInfo{
			TokenID:   tokenId,
			TokenType: tokenType,
			AssetType: newEvent.AssetType,
		}
		err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
		if err != nil {
			handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, "", txnBlockOwner, fmt.Sprintf("tokenchian is wrong for the token%s is wrong,error %v", tokenId, err))
			if handled {
				if handleErr != nil {
					return handleErr
				}
				return nil
			}
			return fmt.Errorf("failed to sync the token chain for the token%s", tokenId)
		}
		c.log.Info("execution block processed successfully", "sc hash ", tokenId, "blockHash ", newEvent.BlockHash)
		return nil
	}

	// Validate block sequence continuity
	latestBlockNumber, err := latestTokenBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get latest block number for contract: %v", err)
	}
	previousOwner := latestTokenBlock.GetOwner()

	currentBlockNumber, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		return fmt.Errorf("failed to get current block number for contract: %v", err)
	}
	p, err := c.getPeer(newEvent.PublisherDID)
	if err != nil {
		c.log.Error("failed to sync full token chain, failed to open peer connection with publisher ", newEvent.PublisherDID)
		return fmt.Errorf("failed to open peer connection with publisher %s", newEvent.PublisherDID)
	}
	defer p.Close()

	// Check for missing blocks in contract chain
	if latestBlockNumber+1 != currentBlockNumber {
		if latestBlockNumber == currentBlockNumber {
			latestBlockHash, _ := latestTokenBlock.GetHash()
			if newEvent.BlockHash == latestBlockHash {
				return nil
			}
			//add this token to double spend tokens table
			doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
				TokenID:        tokenId,
				AssetType:      newEvent.AssetType,
				TokenType:      tokenType,
				PublisherDID:   newEvent.PublisherDID,
				ClaimedOwnerI:  previousOwner,
				ClaimedOwnerII: txnBlockOwner,
				ErrorMessage:   fmt.Sprintf("block with the same block number exist, dual ownership issue or same did would have double spent the token, tokenID:%s", tokenId),
			}
			err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
			if err != nil {
				errMsg := doubleSpentTokenInfo.ErrorMessage + "failed to update double spent token in tables"
				return fmt.Errorf("%v", errMsg)
			}

			return fmt.Errorf("block with the same block number exist, dual ownership issue or same did would have double spent the token, tokenID:%s", tokenId)
		} else if latestBlockNumber < currentBlockNumber {
			//connect to publisher and fetch complete token chain

			tokenSyncInfo := &TokenSyncInfo{
				TokenID:   tokenId,
				TokenType: tokenType,
				AssetType: newEvent.AssetType,
			}
			err = c.SyncFullTokenChainForFullNode(p, *tokenSyncInfo)
			if err != nil {
				handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, previousOwner, txnBlockOwner, fmt.Sprintf("%s, %s both dids are claiming the same token or same did would have double spent the token,dual ownership issue", previousOwner, txnBlockOwner))
				if handled {
					if handleErr != nil {
						return handleErr
					}
					return nil
				}
				c.log.Error("Failed to sync chain", "token", tokenId, "err", err)
				info := &model.FailedToSyncTokenDetailsInfo{
					TokenID:   tokenId,
					TokenType: tokenType,
					AssetType: newEvent.AssetType,
					Did:       newEvent.PublisherDID,
					Reason:    fmt.Sprintf("failed to sync chain,err: %v", err),
				}

				if err := c.w.AddFailedTokensToTable(info); err != nil {
					c.log.Error("Failed to record failed token sync in DB", "token", tokenId, "error", err)
				} else {
					c.log.Info("Recorded failed token sync in DB", "token", tokenId)
					return nil
				}
			}
			// check if token exists in postgres table, add if doesn't
			err := c.ReadTokenContentFromPSQL(tokenId, newEvent.AssetType)
			if err != nil {
				if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
					c.log.Error("failed to add token's ipfs content to psql db, err: %v", err)
				}
			}

			c.log.Info("Transfer transaction processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
			return nil
			// return fmt.Errorf("mismatch of blocks detected: latest block number=%d, current block number=%d", latestBlockNumber, currentBlockNumber)
		} else {
			incomingBlockID := txnBlockID
			publishersLatestBlockNumber := currentBlockNumber
			//get Fullnode block whose block number is publisher side latest blockID
			fullnodesideBlockBytes, err := c.w.GetFullNodeTokenBlockByNumber(tokenId, tokenType, publishersLatestBlockNumber)
			fullnodesideBlock := block.InitBlock(fullnodesideBlockBytes, nil)
			fullnodesideBlockID, err := fullnodesideBlock.GetBlockID(tokenId)
			if err != nil {
				c.log.Error("failed to get blockID of the fullnode side block", "error", err)
			}
			fullnodesideBlockOwner := fullnodesideBlock.GetOwner()
			if incomingBlockID != fullnodesideBlockID {
				//add token to double spend tokens table, with an error saying
				//that both DID1 or DID2 are claimming the same token.
				doubleSpentTokenInfo := &model.DoubleSpentTokenInfo{
					TokenID:        tokenId,
					AssetType:      newEvent.AssetType,
					TokenType:      tokenType,
					PublisherDID:   newEvent.PublisherDID,
					ClaimedOwnerI:  fullnodesideBlockOwner,
					ClaimedOwnerII: txnBlockOwner,
					ErrorMessage:   fmt.Sprintf("%s, %s both dids are claiming the same token,dual ownership issue", fullnodesideBlockOwner, txnBlockOwner),
				}
				// store double spent token info in DoubleSpentTokens table, and remove it from respective tokens table
				err = c.StoreDoubleSpentTokenInfo(doubleSpentTokenInfo)
				if err != nil {
					errMsg := fmt.Sprintf("failed to update double spent token : %v, err: %v", tokenId, err)
					c.log.Error(errMsg)
				}
				c.log.Error("token has been added to double spend tokens table, tokenID", tokenId)
				return nil

			} else {
				c.log.Info("current transaction blk %salready exist with fullnode", txnBlockID)
				return nil
			}

		}
	}

	// deployer should be the contract owner for all smart contracts, but for NFTs, owners keep changing
	if newEvent.AssetType == NFTTokenType {
		if previousOwner != newEvent.PublisherDID {
			return fmt.Errorf("NFT publisher DID mismatch: expected %s, got %s", previousOwner, newEvent.PublisherDID)
		}
	}

	// Do all 3 checks, If any check fails, handle the failure here also just before adding the block to leveldb.
	err = c.ValidateIncomingTokenBlock(*txnBlock, latestTokenBlock, tokenId, p, newEvent.AssetType)
	if err != nil {
		handled, handleErr := c.HandleSyncErrorAsDoubleSpent(err, tokenId, newEvent.AssetType, tokenType, newEvent.PublisherDID, previousOwner, txnBlockOwner, fmt.Sprintf("Incoming block %s is wrong,error %v", txnBlockID, err))
		if handled {
			if handleErr != nil {
				return handleErr
			}
			return nil
		}
	}
	// Add validated block to contract chain
	if err := c.w.AddFullNodeTokenBlock(tokenId, txnBlock); err != nil {
		return fmt.Errorf("failed to add contract execution block to chain: %v", err)
	}
	currentOwner := txnBlock.GetOwner()
	// update block height if required
	latestBlockHeight, err := txnBlock.GetBlockNumber(tokenId)
	if err != nil {
		c.log.Error("failed to get block height")
	}
	newEvent.LatestBlockHeight = latestBlockHeight
	syncStatus := wallet.SyncCompleted
	receivedBlock := ReceivedBlock{
		LatestBlock: txnBlock,
	}
	genesisBlock := c.w.GetFullNodeGenesisTokenBlock(tokenId, tokenType)
	if genesisBlock != nil {
		receivedBlock.GenesisBlock = genesisBlock
	}

	if err := c.AddTokenToRespectiveTable(tokenId, currentOwner, receivedBlock, newEvent, syncStatus); err != nil {
		return fmt.Errorf("failed to add contract token to table: %v", err)
	}

	// check if token exists in postgres table, add if doesn't
	err = c.ReadTokenContentFromPSQL(tokenId, newEvent.AssetType)
	if err != nil {
		if err := c.AddTokenContentToPSQL(tokenId, newEvent.AssetType); err != nil {
			c.log.Error("failed to add token's ipfs content to psql db, err: %v", err)
			return fmt.Errorf("failed to add ipfs content of the token%s to psql db,err:%v", tokenId, err)
		}
	}

	c.log.Info("Contract execution processed successfully", "tokenId", tokenId, "blockHash", newEvent.BlockHash)
	return nil
}

// Handle failed transactions after all retries
func (c *Core) handleFailedTransaction(txnEvent *model.PubSubTxnInfo, lastErr error) {
	c.log.Error("Transaction processing failed permanently",
		"blockHash", txnEvent.BlockHash,
		"error", lastErr)

	// Implement failure handling strategy:
	// 1. Store in failed transactions table for manual review
	// 2. Trigger alerts
	// 3. Attempt alternative processing paths

	// Example: Store in failed transactions table
	failedTxn := &model.FailedTransaction{
		BlockHash:    txnEvent.BlockHash,
		PublisherDID: txnEvent.PublisherDID,
		Error:        lastErr.Error(),
		FailedAt:     time.Now(),
		RetryCount:   c.txnProcessor.maxRetries,
	}

	if err := c.w.StoreFailedTransaction(failedTxn); err != nil {
		c.log.Error("Failed to store failed transaction", "blockHash", txnEvent.BlockHash, "error", err)
	}
}

// Graceful shutdown
func (c *Core) ShutdownTxnProcessor() {
	if c.txnProcessor != nil {
		c.log.Info("Shutting down transaction processor")
		c.txnProcessor.cancel()

		// Close the queue channel to signal workers to finish current work
		close(c.txnProcessor.txnQueue)

		// Wait for all workers to complete with timeout
		done := make(chan struct{})
		go func() {
			c.txnProcessor.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			c.log.Info("All transaction workers shut down gracefully")
		case <-time.After(30 * time.Second):
			c.log.Warn("Transaction workers shutdown timeout - forcing termination")
		}
	}
}

// This function process incoming transaction history details and add it to Fullnode transaction history table
func (c *Core) processIncomingTransactionHistory(txns []model.FullNodeTxnHistoryInfo) {
	for _, t := range txns {
		err := c.w.AddTransactionsToFullNodeTransactionHistoryTable(&t)
		if err != nil {
			c.log.Error("Failed to store txn history", "txn", t.TransactionID, "err", err)
		}
		// ensuring correct txn amount is stored with fullnode
		storedTxn, err := c.w.ReadFullNodeTransactionHistoryTable(t.TransactionID)
		if err == nil && storedTxn.TransactionValue != t.TransactionValue {
			err = c.w.UpdateFullNodeTransactionHistoryTable(&t)
			if err != nil {
				errMsg := fmt.Sprintf("failed to update transaction amount for transaction id : %v, stored value is : %v and received value is : %v", t.TransactionID, storedTxn.TransactionValue, t.TransactionValue)
				c.log.Error(errMsg)

			}
		}
	}

	c.log.Info("Stored transaction history batch", "count", len(txns))
}
