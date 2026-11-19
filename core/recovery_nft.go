package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

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

// EphemeralNFTID builds the ephemeral NFT ID for a recovery request.
// Like a core child NFT (nft_child_mint.go), it is the IPFS hash of the
// parent NFT ID followed by a suffix; here the parent is the Recovery NFT
// and the suffix is the request transaction ID, so every node gets the same ID.
func (c *Core) EphemeralNFTID(requestTxID string) (string, error) {
	recoveryNFTID := c.RecoveryNFTID()
	if err := util.ValidateCIDFormat(recoveryNFTID); err != nil {
		return "", fmt.Errorf("invalid recovery NFT ID: %w", err)
	}
	if _, err := hex.DecodeString(requestTxID); err != nil || len(requestTxID) != 64 {
		return "", fmt.Errorf("invalid request transaction ID: %s", requestTxID)
	}
	payload := []byte(recoveryNFTID + requestTxID)
	ephemeralID, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to IPFS-add ephemeral NFT payload for request %s: %w", requestTxID, err)
	}
	return ephemeralID, nil
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
	dc, err := c.SetupDID(reqID, request.DID)
	if err != nil {
		return "", err
	}
	txID, err := c.runRecoveryNFT(reqID, dc, request.DID, nftID, string(data))
	if err != nil {
		return "", err
	}
	// This node gets no callback for its own execution, so dispatch here.
	c.dispatchRecoveryEvent(nftID, txID, "")
	return txID, nil
}

// DeployEphemeralNFT deploys the ephemeral NFT for a recovery request.
// If the ephemeral NFT already exists on this node, it does nothing when did
// deployed it, and fails when another DID did, because that blocks the recovery.
// It is a plain deploy, so the Recovery NFT is not run again.
func (c *Core) DeployEphemeralNFT(reqID, did, requestTxID string) error {
	ephemeralID, err := c.EphemeralNFTID(requestTxID)
	if err != nil {
		return err
	}
	exists, err := c.w.TokenExists(c.Ctx, nil, ephemeralID)
	if err != nil {
		return fmt.Errorf("failed to check ephemeral NFT %s: %w", ephemeralID, err)
	}
	if exists {
		chain, err := c.recoveryChain(ephemeralID)
		if err != nil {
			return fmt.Errorf("failed to read ephemeral NFT %s: %w", ephemeralID, err)
		}
		if len(chain) > 0 && chain[0].Info.Initiator != did {
			return fmt.Errorf("recovery blocked: ephemeral NFT %s was deployed by %s", ephemeralID, chain[0].Info.Initiator)
		}
		return nil
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return err
	}
	txID, err := c.runRecoveryNFT(reqID, dc, did, ephemeralID, "")
	if err != nil {
		return err
	}
	c.dispatchRecoveryEvent(ephemeralID, txID, "")
	return nil
}

// ExecuteEphemeralNFT puts a recovery message on the ephemeral NFT of the
// given request. The sender DID runs the transaction.
func (c *Core) ExecuteEphemeralNFT(reqID, requestTxID string, msg *types.RecoveryMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("recovery message is required")
	}
	dc, err := c.SetupDID(reqID, msg.SenderDID)
	if err != nil {
		return "", err
	}
	return c.executeEphemeralNFT(reqID, dc, requestTxID, msg)
}

// executeEphemeralNFT is ExecuteEphemeralNFT with the sender's DID already
// set up, for senders that sign without an API request.
func (c *Core) executeEphemeralNFT(reqID string, dc types.DIDCrypto, requestTxID string, msg *types.RecoveryMessage) (string, error) {
	ephemeralID, err := c.EphemeralNFTID(requestTxID)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to encode recovery message: %w", err)
	}
	txID, err := c.runRecoveryNFT(reqID, dc, msg.SenderDID, ephemeralID, string(data))
	if err != nil {
		return "", err
	}
	c.dispatchRecoveryEvent(ephemeralID, txID, "")
	return txID, nil
}

// runRecoveryNFT runs one NFT transaction signed by dc and returns its
// transaction ID. The NFT is deployed if this node does not have it, else it
// is executed.
func (c *Core) runRecoveryNFT(reqID string, dc types.DIDCrypto, did, nftID, data string) (string, error) {
	resp := c.initiateTransactionWithDID(reqID, dc, &models.TransactionRequest{
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

// SubscribeRecoveryNFT subscribes to the Recovery NFT topic at startup and
// restores open recovery sessions. It does nothing when recovery is off on
// this network.
func (c *Core) SubscribeRecoveryNFT() {
	if nftID := c.RecoveryNFTID(); nftID != "" {
		c.subscribeRecoveryTopic(nftID)
		c.restoreRecoverySessions()
	}
}

// subscribeRecoveryTopic subscribes to a recovery NFT topic. It only
// subscribes; the ephemeral NFT may not be deployed yet, so nothing is fetched.
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
