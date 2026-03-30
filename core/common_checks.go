package core

// import (
// 	"encoding/json"
// 	"fmt"

// 	"github.com/rubixchain/rubixgoplatform/core/consensus"
// 	"github.com/rubixchain/rubixgoplatform/types"
// 	"github.com/rubixchain/rubixgoplatform/types/models"
// )

// func (c *Core) ValidateTokenOwnershipByPrevTxn(txnInfo *models.TransactionInfo) error {
// 	return consensus.ValidateTokenOwnershipByPrevTxn(txnInfo, c.w)
// }

// func (c *Core) ValidateTransactionInfoFields(txnInfo *models.TransactionInfo) error {
// 	return consensus.ValidateTransactionInfoFields(txnInfo)
// }

// func (c *Core) TransactionIDIntegrityCheck(transactionID string, transactionInfo *models.TransactionInfo) error {
// 	return consensus.TransactionIDIntegrityCheck(transactionID, transactionInfo)
// }

// func (c *Core) SignatureVerificationCheck(tx *models.Transactions) error {
// 	var txInfo models.TransactionInfo
// 	if err := json.Unmarshal(tx.Info, &txInfo); err != nil {
// 		return fmt.Errorf("SignatureVerificationCheck: failed to unmarshal transaction info: %w", err)
// 	}

// 	var sig models.Signature
// 	if err := json.Unmarshal(tx.Signature, &sig); err != nil {
// 		return fmt.Errorf("SignatureVerificationCheck: failed to unmarshal signature: %w", err)
// 	}

// 	initiatorDC, err := c.InitialiseDID(txInfo.Initiator)
// 	if err != nil {
// 		return fmt.Errorf("SignatureVerificationCheck: failed to initialise initiator DID %s: %w", txInfo.Initiator, err)
// 	}

// 	quorumDCs := make(map[string]types.DIDCrypto, len(sig.Quorums))
// 	for _, quorumSig := range sig.Quorums {
// 		dc, err := c.InitialiseDID(quorumSig.Did)
// 		if err != nil {
// 			return fmt.Errorf("SignatureVerificationCheck: failed to initialise quorum DID %s: %w", quorumSig.Did, err)
// 		}
// 		quorumDCs[quorumSig.Did] = dc
// 	}

// 	return consensus.SignatureVerificationCheck(&txInfo, &sig, initiatorDC, quorumDCs)
// }

// func (c *Core) ValidateNewTokenContent(tokenID string, isQuorum bool) error {
// 	return consensus.ValidateNewTokenContent(tokenID, isQuorum, c.testnet, c.mainnet, c.localnet, c.log)
// }

// func (c *Core) ValidateTransactionValueAndPledge(txnInfo *models.TransactionInfo) error {
// 	return consensus.ValidateTransactionValueAndPledge(txnInfo)
// }

// func (c *Core) IsParentTokenBurnt(isFullNode bool, tokenID string) (error, bool) {
// 	return consensus.IsParentTokenBurnt(isFullNode, tokenID, c.w)
// }

// func (c *Core) ValidateTokenIDRelatedChecks(tokenID string, isFullNode bool) error {
// 	return consensus.ValidateTokenIDRelatedChecks(tokenID, isFullNode, c.w, c.testnet, c.mainnet, c.localnet, c.log)
// }

// func (c *Core) ValidateGenuineTokenCreator(tokenID string, isFullNode bool) error {
// 	return consensus.ValidateGenuineTokenCreator(tokenID, isFullNode, c.w)
// }

// func (c *Core) TokenChainIntigrityCheck(txnInfo *models.TransactionInfo) (error, bool) {
// 	peer, err := c.getPeer(txnInfo.Initiator)
// 	if err != nil {
// 		return fmt.Errorf("TokenChainIntigrityCheck: failed to get peer: %w", err), false
// 	}
// 	defer peer.Close()
// 	return consensus.TokenChainIntigrityCheck(txnInfo, peer, c.w, c.log)
// }

// func (c *Core) ValidateIPFSPinChecks(txnInfo *models.TransactionInfo, isFullnode bool) error {
// 	return consensus.ValidateIPFSPinChecks(txnInfo, isFullnode, c.checkTokenStateHashPinned)
// }

// func (c *Core) checkTokenStateHashPinned(tokenID string, previousTransactionID string) error {
// 	if previousTransactionID == "" {
// 		return nil
// 	}

// 	tokenStateHash := tokenID + "." + previousTransactionID

// 	record, err := c.ipfsProviderStore.GetProviderByCID(tokenStateHash)
// 	if err != nil {
// 		return fmt.Errorf("failed to check pin status for %s: %w", tokenStateHash, err)
// 	}
// 	if record != nil {
// 		return fmt.Errorf("token %s is already pinned", tokenStateHash)
// 	}

// 	return nil
// }

// func (c *Core) ValidateTransaction(tx *models.Transactions, isFullnode bool) (bool, error) {
// 	var txnInfo models.TransactionInfo
// 	if err := json.Unmarshal(tx.Info, &txnInfo); err != nil {
// 		return false, fmt.Errorf("ValidateTransaction: failed to unmarshal transaction info: %w", err)
// 	}

// 	initiatorDC, err := c.InitialiseDID(txnInfo.Initiator)
// 	if err != nil {
// 		return false, fmt.Errorf("ValidateTransaction: failed to initialise initiator DID %s: %w", txnInfo.Initiator, err)
// 	}

// 	var sig models.Signature
// 	if err := json.Unmarshal(tx.Signature, &sig); err != nil {
// 		return false, fmt.Errorf("ValidateTransaction: failed to unmarshal signature: %w", err)
// 	}

// 	quorumDCs := make(map[string]types.DIDCrypto, len(sig.Quorums))
// 	for _, quorumSig := range sig.Quorums {
// 		dc, err := c.InitialiseDID(quorumSig.Did)
// 		if err != nil {
// 			return false, fmt.Errorf("ValidateTransaction: failed to initialise quorum DID %s: %w", quorumSig.Did, err)
// 		}
// 		quorumDCs[quorumSig.Did] = dc
// 	}

// 	peer, err := c.getPeer(txnInfo.Initiator)
// 	if err != nil {
// 		return false, fmt.Errorf("ValidateTransaction: failed to get peer: %w", err)
// 	}
// 	defer peer.Close()

// 	return consensus.ValidateTransaction(
// 		tx,
// 		isFullnode,
// 		c.w,
// 		c.log,
// 		initiatorDC,
// 		quorumDCs,
// 		peer,
// 		c.testnet,
// 		c.mainnet,
// 		c.localnet,
// 		c.checkTokenStateHashPinned,
// 	)
// }
