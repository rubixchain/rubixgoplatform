package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// recoveryChildSuffixLen is how many characters of the request
// transaction ID go into the child NFT ID (96 bits).
const recoveryChildSuffixLen = 24

// RecoveryNFTID returns the Recovery NFT ID for this node's network.
// On localnet it comes from config; an empty result means recovery is off.
func (c *Core) RecoveryNFTID() string {
	switch {
	case c.mainnet:
		return constants.RecoveryNFTID_Mainnet
	case c.testnet:
		return constants.RecoveryNFTID_Testnet
	case c.localnet:
		return c.cfg.LocalnetRecoveryNFTID
	}
	return ""
}

// RecoveryChildNFTID builds the child NFT ID for a recovery request.
// It keeps the first 22 characters of the Recovery NFT ID and adds the
// last 24 characters of the request transaction ID, so the result is
// still a 46-character CID that every node can work out on its own.
func RecoveryChildNFTID(recoveryNFTID, requestTxID string) (string, error) {
	if err := util.ValidateCIDFormat(recoveryNFTID); err != nil {
		return "", fmt.Errorf("invalid recovery NFT ID: %w", err)
	}
	if _, err := hex.DecodeString(requestTxID); err != nil || len(requestTxID) != 64 {
		return "", fmt.Errorf("invalid request transaction ID: %s", requestTxID)
	}
	keep := len(recoveryNFTID) - recoveryChildSuffixLen
	return recoveryNFTID[:keep] + requestTxID[len(requestTxID)-recoveryChildSuffixLen:], nil
}

// ExecuteRecoveryNFT puts a signed recovery request on the Recovery NFT.
// It returns the transaction ID, which is the recovery request ID.
func (c *Core) ExecuteRecoveryNFT(reqID string, request *types.RecoveryRequest) (string, error) {
	nftID := c.RecoveryNFTID()
	if nftID == "" {
		return "", fmt.Errorf("recovery is off: no Recovery NFT ID is set for this network")
	}
	if request == nil {
		return "", fmt.Errorf("recovery request is required")
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to encode recovery request: %w", err)
	}
	txID, err := c.runRecoveryNFT(reqID, request.DID, nftID, string(data))
	if err != nil {
		return "", err
	}
	// This node gets no callback for its own execution, so subscribe here.
	c.subscribeRecoveryChild(txID)
	return txID, nil
}

// DeployRecoveryChild deploys the child NFT for a recovery request.
// It does nothing if the child already exists on this node.
// It is a plain deploy, so the Recovery NFT is not run again.
func (c *Core) DeployRecoveryChild(reqID, did, requestTxID string) error {
	childID, err := RecoveryChildNFTID(c.RecoveryNFTID(), requestTxID)
	if err != nil {
		return err
	}
	exists, err := c.w.TokenExists(c.Ctx, nil, childID)
	if err != nil {
		return fmt.Errorf("failed to check child NFT %s: %w", childID, err)
	}
	if exists {
		return nil
	}
	_, err = c.runRecoveryNFT(reqID, did, childID, "")
	return err
}

// ExecuteRecoveryChild puts a recovery message on the child NFT of the
// request named in msg.RequestID. The sender DID runs the transaction.
func (c *Core) ExecuteRecoveryChild(reqID string, msg *types.RecoveryMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("recovery message is required")
	}
	childID, err := RecoveryChildNFTID(c.RecoveryNFTID(), msg.RequestID)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to encode recovery message: %w", err)
	}
	return c.runRecoveryNFT(reqID, msg.SenderDID, childID, string(data))
}

// runRecoveryNFT runs one NFT transaction and returns its transaction ID.
// The NFT is deployed if this node does not have it, else it is executed.
func (c *Core) runRecoveryNFT(reqID, did, nftID, data string) (string, error) {
	resp := c.initiateTransaction(reqID, &models.TransactionRequest{
		Initiator: did,
		Tokens: models.TransactionTokenDetails{
			NFT: []models.NFTInfo{{NFTId: nftID, Data: data}},
		},
	})
	if !resp.Status {
		return "", fmt.Errorf("recovery NFT %s: %s", nftID, resp.Message)
	}
	result, ok := resp.Result.(models.TransactionResult)
	if !ok {
		return "", fmt.Errorf("recovery NFT %s: no transaction ID in the result", nftID)
	}
	return result.TransactionID, nil
}

// SubscribeRecoveryNFT subscribes to the Recovery NFT topic at startup.
// It does nothing when recovery is off on this network.
func (c *Core) SubscribeRecoveryNFT() {
	if nftID := c.RecoveryNFTID(); nftID != "" {
		c.subscribeRecoveryTopic(nftID)
	}
}

// subscribeRecoveryChild subscribes to the child NFT of a recovery request.
func (c *Core) subscribeRecoveryChild(requestTxID string) {
	childID, err := RecoveryChildNFTID(c.RecoveryNFTID(), requestTxID)
	if err != nil {
		c.log.Error("Failed to derive recovery child NFT ID", "request_txn_id", requestTxID, "err", err)
		return
	}
	c.subscribeRecoveryTopic(childID)
}

// subscribeRecoveryTopic subscribes to a recovery NFT topic. It only
// subscribes; the child may not be deployed yet, so nothing is fetched.
func (c *Core) subscribeRecoveryTopic(topic string) {
	if c.ps.IsSubscribed(topic) {
		return
	}
	if err := c.ps.SubscribeTopic(topic, c.NFTCallBack); err != nil && !c.ps.IsSubscribed(topic) {
		c.log.Error("Failed to subscribe to recovery NFT topic", "topic", topic, "err", err)
		return
	}
	c.log.Info("Subscribed to recovery NFT topic", "topic", topic)
}
