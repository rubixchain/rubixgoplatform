package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// RollbackRequest represents a request to rollback a transaction
type RollbackRequest struct {
	TransactionID string `json:"transaction_id"`
	Role          string `json:"role"` // sender, quorum, or receiver
	Reason        string `json:"reason"`
}

// initTransactionRollback initiates a transaction rollback
func (c *Core) initTransactionRollback(req *ensweb.Request) *ensweb.Result {
	var rbr RollbackRequest
	err := c.l.ParseJSON(req, &rbr)
	if err != nil {
		c.log.Error("Failed to parse rollback request", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Failed to parse request",
		}, 400)
	}

	c.log.Info("Initiating transaction rollback",
		"transaction_id", rbr.TransactionID,
		"role", rbr.Role,
		"reason", rbr.Reason)

	// Get transaction state
	txState, err := c.txStateMgr.GetTransactionState(rbr.TransactionID)
	if err != nil {
		c.log.Error("Transaction not found", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Transaction not found",
		}, 404)
	}

	// Check if already rolled back
	if txState.State == TxStateRolledBack {
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  true,
			Message: "Transaction already rolled back",
		}, 200)
	}

	// Execute rollback based on role
	err = c.executeRollback(rbr.TransactionID, rbr.Role)
	if err != nil {
		c.log.Error("Failed to execute rollback", "err", err)
		return c.l.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: fmt.Sprintf("Failed to execute rollback: %v", err),
		}, 500)
	}

	// If this is the sender, coordinate rollback with other participants
	if rbr.Role == "sender" {
		go c.coordinateRollback(txState)
	}

	return c.l.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: "Rollback completed successfully",
	}, 200)
}

// executeRollback executes the rollback for a specific role
func (c *Core) executeRollback(txID string, role string) error {
	c.log.Info("Executing rollback",
		"transaction_id", txID,
		"role", role)

	// Create rollback snapshot
	snapshot, err := c.w.CreateRollbackSnapshot(txID, role)
	if err != nil {
		return fmt.Errorf("failed to create rollback snapshot: %v", err)
	}

	// Execute wallet rollback
	err = c.w.ExecuteRollback(snapshot)
	if err != nil {
		return fmt.Errorf("failed to execute wallet rollback: %v", err)
	}

	// Additional rollback based on role
	switch role {
	case "sender":
		// Remove any consensus data
		c.removeConsensusData(txID)
		
	case "quorum":
		// Remove token state hash entries
		c.removeTokenStateHashEntries(txID)
		
	case "receiver":
		// Clean up any IPFS pins
		c.cleanupIPFSPins(txID)
	}

	// Update transaction state
	err = c.txStateMgr.UpdateState(txID, TxStateRolledBack)
	if err != nil {
		c.log.Error("Failed to update transaction state", "err", err)
	}

	return nil
}

// coordinateRollback coordinates rollback across all participants
func (c *Core) coordinateRollback(txState *TransactionStateInfo) {
	c.log.Info("Coordinating rollback across participants",
		"transaction_id", txState.TransactionID)

	// Rollback on receiver
	if txState.Participants.Receiver != "" {
		err := c.sendRollbackRequest(txState.Participants.Receiver, txState.TransactionID, "receiver")
		if err != nil {
			c.log.Error("Failed to rollback on receiver",
				"receiver", txState.Participants.Receiver,
				"error", err)
		}
	}

	// Rollback on quorums
	for _, quorum := range txState.Participants.Quorums {
		err := c.sendRollbackRequest(quorum, txState.TransactionID, "quorum")
		if err != nil {
			c.log.Error("Failed to rollback on quorum",
				"quorum", quorum,
				"error", err)
		}
	}
}

// sendRollbackRequest sends a rollback request to a participant
func (c *Core) sendRollbackRequest(participant string, txID string, role string) error {
	c.log.Info("Sending rollback request",
		"participant", participant,
		"transaction_id", txID,
		"role", role)

	peer, err := c.getPeer(participant)
	if err != nil {
		return fmt.Errorf("failed to get peer: %v", err)
	}
	defer peer.Close()

	rbr := RollbackRequest{
		TransactionID: txID,
		Role:          role,
		Reason:        "Transaction failed - initiating rollback",
	}

	var resp model.BasicResponse
	err = peer.SendJSONRequest("POST", APIRollbackTransaction, nil, &rbr, &resp, true)
	if err != nil {
		return fmt.Errorf("failed to send rollback request: %v", err)
	}

	if !resp.Status {
		return fmt.Errorf("rollback failed on participant: %s", resp.Message)
	}

	return nil
}

// removeConsensusData removes consensus-related data for a transaction
func (c *Core) removeConsensusData(txID string) {
	c.log.Info("Removing consensus data",
		"transaction_id", txID)

	// TODO: Remove from consensus request storage when available
	// key := fmt.Sprintf("consensus:%s", txID)
	// err := c.s.Delete(key)
	// if err != nil {
	//     c.log.Error("Failed to remove consensus data", "err", err)
	// }
}

// removeTokenStateHashEntries removes token state hash entries
func (c *Core) removeTokenStateHashEntries(txID string) {
	c.log.Info("Removing token state hash entries",
		"transaction_id", txID)

	// This would need to iterate through TokenStateHashTable
	// and remove entries related to this transaction
	// Implementation depends on the specific storage structure
}

// cleanupIPFSPins cleans up IPFS pins created for the transaction
func (c *Core) cleanupIPFSPins(txID string) {
	c.log.Info("Cleaning up IPFS pins",
		"transaction_id", txID)

	// This would unpin any IPFS objects created for this transaction
	// Implementation depends on how pins are tracked
}

// RollbackManager handles automatic rollback for failed transactions
type RollbackManager struct {
	core        *Core
	txStateMgr  *TransactionStateManager
	rollbackCh  chan string
	stopCh      chan struct{}
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(core *Core, txStateMgr *TransactionStateManager) *RollbackManager {
	return &RollbackManager{
		core:       core,
		txStateMgr: txStateMgr,
		rollbackCh: make(chan string, 100),
		stopCh:     make(chan struct{}),
	}
}

// Start starts the rollback manager
func (rm *RollbackManager) Start() {
	go rm.processRollbacks()
	go rm.monitorTimeouts()
}

// Stop stops the rollback manager
func (rm *RollbackManager) Stop() {
	close(rm.stopCh)
}

// QueueRollback queues a transaction for rollback
func (rm *RollbackManager) QueueRollback(txID string) {
	select {
	case rm.rollbackCh <- txID:
		rm.core.log.Info("Queued transaction for rollback", "transaction_id", txID)
	default:
		rm.core.log.Warn("Rollback queue full", "transaction_id", txID)
	}
}

// processRollbacks processes queued rollbacks
func (rm *RollbackManager) processRollbacks() {
	for {
		select {
		case txID := <-rm.rollbackCh:
			rm.handleRollback(txID)
		case <-rm.stopCh:
			return
		}
	}
}

// handleRollback handles a single rollback
func (rm *RollbackManager) handleRollback(txID string) {
	rm.core.log.Info("Processing rollback", "transaction_id", txID)

	txState, err := rm.txStateMgr.GetTransactionState(txID)
	if err != nil {
		rm.core.log.Error("Failed to get transaction state", "err", err)
		return
	}

	// Execute rollback on sender first
	err = rm.core.executeRollback(txID, "sender")
	if err != nil {
		rm.core.log.Error("Failed to execute sender rollback", "err", err)
	}

	// Coordinate rollback with other participants
	rm.core.coordinateRollback(txState)
}

// monitorTimeouts monitors for transaction timeouts
func (rm *RollbackManager) monitorTimeouts() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.checkTimeouts()
		case <-rm.stopCh:
			return
		}
	}
}

// checkTimeouts checks for timed out transactions
func (rm *RollbackManager) checkTimeouts() {
	// Get all active transactions
	// Check if any have exceeded timeout
	// Queue them for rollback
	// This would need access to all transaction states
}