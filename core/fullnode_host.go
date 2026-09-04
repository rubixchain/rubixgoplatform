package core

import (
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/fullnode"
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// The node's side of the fullnode ingest pipeline, which lives in core/fullnode.
//
// Two things stay here rather than moving with it. SubscribeTxnSetup and
// TxnCallBack run on every node including a quorum, and do quorum unpledge and
// peer-table work that is not fullnode business. checkTokenStateHashPinned is
// passed to ValidateTransaction by the quorum path too (quorum_initiator.go).
//
// Everything else the pipeline needs from the node arrives through
// fullnode.Host, which Core implements below.

// Enhanced subscription setup with error handling
func (c *Core) SubscribeTxnSetup() {
	// Only fullnodes serve these endpoints
	if c.fullNode {
		c.txnProcessor = fullnode.NewTxnProcessor(c)
		c.txnProcessor.RegisterRoutes()
	}

	topic := constants.Event_RubixTxns
	err := c.ps.SubscribeTopic(topic, c.TxnCallBack)
	if err != nil {
		// If already subscribed, this is expected when SetupQuorum is called
		// for multiple quorum DIDs on the same node. Not an error.
		if err.Error() == "topic already subscribed" {
			c.log.Debug("SubscribeTxnSetup: already subscribed to topic, skipping", "topic", topic)
			return
		}
		c.log.Error("Unable to subscribe to topic", "topic", topic, "error", err)
		return
	}
	c.log.Info("Successfully subscribed to topic: " + topic)
}

// Enhanced callback with dynamic scaling integration
func (c *Core) TxnCallBack(peerID string, topic string, data []byte) {
	var newEvent models.EventTransaction
	err := json.Unmarshal(data, &newEvent)
	if err != nil {
		c.log.Error("Failed to parse published event", "error", err, "data", string(data))
		return
	}

	// Ignore failed transaction IDs
	// Valid condition for both full node and Quorum nodes
	if !newEvent.Status {
		return
	}

	var txInfo models.TransactionInfo
	if err := json.Unmarshal(newEvent.Transaction.Info, &txInfo); err != nil {
		c.log.Error(fmt.Sprintf("failed to unmarshal transaction info, err: %v", err))
		return
	}

	// If the current node is setup as quorum, we check the records in token_state_hashes
	// table to see if any previous transaction id from TransactionInfo is present in the
	// current node's table. If so, then its removed
	if len(c.qc) > 0 {
		// Use the first quorum DID registered on this node for unpledge callback
		var quorumDID string
		for did := range c.qc {
			quorumDID = did
			break
		}
		if err := c.CallBackQuorumUnpledge(newEvent.Transaction, quorumDID); err != nil {
			c.log.Error(fmt.Sprintf("failed to check token state hashes records, err: %v", err))
		}
	}

	// add publisher to peer did table
	publisherDetails := models.DID{
		DID:    txInfo.Initiator,
		PeerID: peerID,
		Local:  peerID == c.peerID, // This was empty and was getting updated to false by default, so we set it based on the peerID comparison
	}
	err = c.AddPeerDetails(publisherDetails)
	if err != nil {
		c.log.Error("failed to add publisher info to DB", "err", err)
	}

	if c.fullNode {
		c.txnProcessor.QueueFullnodeTransaction(&newEvent)
	}
}

// Graceful shutdown
func (c *Core) ShutdownTxnProcessor() {
	c.txnProcessor.ShutdownTxnProcessor()
}

func (c *Core) checkTokenStateHashPinned(tokenID string, previousTransactionID string) error {
	if previousTransactionID == "" {
		return nil
	}

	tokenStateHash := tokenID + "." + previousTransactionID

	record, err := c.ipfsProviderStore.GetProviderByCID(tokenStateHash)
	if err != nil {
		return fmt.Errorf("failed to check pin status for %s: %w", tokenStateHash, err)
	}

	if record != nil {
		return fmt.Errorf("token %s is already pinned", tokenStateHash)
	}

	return nil
}

// Core implements fullnode.Host. Every method below is a thin adapter over
// existing node state or an existing method; the assertion keeps the two in step
// so a missing one fails the build here rather than at a call site.
var _ fullnode.Host = (*Core)(nil)

func (c *Core) Log() logger.Logger           { return c.log }
func (c *Core) Wallet() *wallet.Wallet       { return c.w }
func (c *Core) Listener() *ipfsport.Listener { return c.l }
func (c *Core) IsFullNode() bool             { return c.fullNode }

func (c *Core) NetworkFlags() (testnet, mainnet, localnet bool) {
	return c.testnet, c.mainnet, c.localnet
}

// The remaining four wrap unexported methods, which another package cannot see.

func (c *Core) SyncTokensFromFullnode(tokenIDs []string) (map[string]string, error) {
	return c.syncTokensFromFullnode(tokenIDs)
}

func (c *Core) GetTransactionInfoByID(txID string) (*models.TransactionInfo, error) {
	return c.getTransactionInfoByID(txID)
}

func (c *Core) GetParentBurnTxID(parentID string) (string, bool, error) {
	return c.getParentBurnTxID(parentID)
}

func (c *Core) CheckTokenStateHashPinned(tokenID, previousTransactionID string) error {
	return c.checkTokenStateHashPinned(tokenID, previousTransactionID)
}

func (c *Core) CPUUsage(lastStats map[string]uint64) (float64, map[string]uint64) {
	return c.getCPUUsageLinux(lastStats)
}

// ResourceMonitor is stateless, so this allocates nothing worth caching on Core.
func (c *Core) MemoryUsagePercent() float64 {
	var rm ResourceMonitor
	return rm.MemoryUsagePercent()
}

// InitialiseDID, SyncTransactionChainsFromPeer and FetchGenesisTransactionFromPeer
// already match the Host signatures, so they need no adapter here.
