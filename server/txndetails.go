package server

import (
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Recover lost tokens
// @Description Allows senders to recover tokens that were sent but not received by the receiver
// @ID recover-lost-tokens
// @Tags FT
// @Accept json
// @Produce json
// @Param sender_did body string true "DID of the sender"
// @Param transaction_id body string true "Transaction ID to recover tokens from"
// @Success 200 {object} model.BasicResponse
// @Router /api/recover-lost-tokens [post]
func (s *Server) APIRecoverLostTokens(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	recoveryReq := struct {
		SenderDID     string `json:"sender_did"`
		TransactionID string `json:"transaction_id"`
	}{}

	// Parse JSON from request body since we're not using auth middleware
	err := s.ParseJSON(req, &recoveryReq)
	if err != nil {
		// Fallback to checking req.Data if ParseJSON fails
		if req.Data != nil {
			if senderDID, ok := req.Data["sender_did"].(string); ok {
				recoveryReq.SenderDID = senderDID
			}
			if transactionID, ok := req.Data["transaction_id"].(string); ok {
				recoveryReq.TransactionID = transactionID
			}
		}
	}

	// Validate request
	if recoveryReq.SenderDID == "" {
		return s.BasicResponse(req, false, "Sender DID is required", nil)
	}

	if recoveryReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	// Validate DID format
	if !strings.HasPrefix(recoveryReq.SenderDID, "bafybmi") || len(recoveryReq.SenderDID) != 59 {
		return s.BasicResponse(req, false, "Invalid sender DID format", nil)
	}

	s.log.Info("Received token recovery request",
		"sender_did", recoveryReq.SenderDID,
		"transaction_id", recoveryReq.TransactionID)

	// Perform token recovery (same for both local and remote)
	recoveryResult, err := s.c.RecoverLostTokens(recoveryReq.SenderDID, recoveryReq.TransactionID)
	if err != nil {
		s.log.Error("Failed to recover lost tokens",
			"sender_did", recoveryReq.SenderDID,
			"transaction_id", recoveryReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Token recovery failed: "+err.Error(), nil)
	}

	s.log.Info("Token recovery completed successfully",
		"sender_did", recoveryReq.SenderDID,
		"transaction_id", recoveryReq.TransactionID,
		"recovered_count", recoveryResult.RecoveredTokenCount)

	return s.BasicResponse(req, true, "Token recovery completed successfully", recoveryResult)
}

// @Summary Initiate remote token recovery
// @Description Allows Node A to trigger token recovery on Node B
// @ID remote-recover-tokens
// @Tags FT
// @Accept json
// @Produce json
// @Param target_did body string true "DID of the target node where recovery should happen"
// @Param transaction_id body string true "Transaction ID to recover"
// @Param requester_did body string true "DID of the requesting node"
// @Param reason body string false "Reason for remote recovery"
// @Success 200 {object} model.BasicResponse
// @Router /api/remote-recover-tokens [post]
func (s *Server) APIRemoteRecoverTokens(req *ensweb.Request) *ensweb.Result {
	// Parse the remote recovery request
	var remoteReq model.RemoteRecoveryRequest
	err := s.ParseJSON(req, &remoteReq)
	if err != nil {
		s.log.Error("Failed to parse remote recovery request", "error", err)
		return s.BasicResponse(req, false, "Invalid request format", nil)
	}

	// Validate required fields
	if remoteReq.TargetDID == "" {
		return s.BasicResponse(req, false, "Target DID is required", nil)
	}
	if remoteReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	// If requester DID is not provided, use the current node's DID
	if remoteReq.RequesterDID == "" {
		// Get the current node's DID
		remoteReq.RequesterDID = s.c.GetPeerID()
	}

	// Set a default reason if not provided
	if remoteReq.Reason == "" {
		remoteReq.Reason = "Remote recovery requested"
	}

	s.log.Info("Initiating remote token recovery",
		"target_did", remoteReq.TargetDID,
		"transaction_id", remoteReq.TransactionID,
		"requester_did", remoteReq.RequesterDID,
		"reason", remoteReq.Reason)

	// Use the new RemoteRecoverTokens that works like update-status with peer connection
	recoveryResult, err := s.c.RemoteRecoverTokens(&remoteReq)
	if err != nil {
		s.log.Error("Remote token recovery failed",
			"target_did", remoteReq.TargetDID,
			"transaction_id", remoteReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Remote recovery failed: "+err.Error(), nil)
	}

	s.log.Info("Remote token recovery completed successfully",
		"target_did", remoteReq.TargetDID,
		"transaction_id", remoteReq.TransactionID,
		"recovered_count", recoveryResult.RecoveredTokenCount)

	// Return the recovery result
	response := map[string]interface{}{
		"transaction_id":        recoveryResult.TransactionID,
		"recovered_token_count": recoveryResult.RecoveredTokenCount,
		"recovery_date":         recoveryResult.RecoveryDate,
		"status":                recoveryResult.Status,
		"message":               recoveryResult.Message,
		"target_did":            remoteReq.TargetDID,
	}

	return s.BasicResponse(req, true, "Remote recovery completed successfully", response)
}
