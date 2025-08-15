package server

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Get transaction details by Transcation ID
// @Description Retrieves the details of a transaction based on its ID.
// @ID get-txn-details-by-id
// @Tags         Account
// @Accept json
// @Produce json
// @Param txnID query string true "The ID of the transaction to retrieve"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-by-txnId [get]
func (s *Server) APIGetTxnByTxnID(req *ensweb.Request) *ensweb.Result {
	txnID := s.GetQuerry(req, "txnID")
	res, err := s.c.GetTxnDetailsByID(txnID)
	if err != nil {
		if err.Error() == "no records found" {
			s.log.Info("There are no records present for this Transaction ID " + txnID)
			td := model.TxnDetails{
				BasicResponse: model.BasicResponse{
					Status:  true,
					Message: "no records present for this Transaction ID : " + txnID,
				},
				TxnDetails: make([]model.TransactionDetails, 0),
			}
			return s.RenderJSON(req, &td, http.StatusOK)
		}
		s.log.Error("err", err)
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
			},
			TxnDetails: make([]model.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
		},
		TxnDetails: make([]model.TransactionDetails, 0),
	}
	td.TxnDetails = append(td.TxnDetails, res)

	return s.RenderJSON(req, &td, http.StatusOK)
}

// @Summary Get transaction details by dID
// @Description Retrieves the details of a transaction based on DID and date range (on and after start date and before end date).
// @ID get-by-did
// @Tags Account
// @Accept json
// @Produce json
// @Param DID query string true "DID of sender/receiver"
// @Param Role query string false "Filter by role as sender or receiver"
// @Param StartDate query string false "Start date of the date range (format: YYYY-MM-DD)"
// @Param EndDate query string false "End date of the date range (format: YYYY-MM-DD)"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-by-did [get]
func (s *Server) APIGetTxnByDID(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "DID")
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	role := s.GetQuerry(req, "Role")
	startDate := s.GetQuerry(req, "StartDate")
	endDate := s.GetQuerry(req, "EndDate")

	res, err := s.c.GetTxnDetailsByDID(did, role, startDate, endDate)
	if err != nil {
		s.log.Info("Error fetching transaction details. " + err.Error())
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
				Result:  "No data found",
			},
			TxnDetails: make([]model.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
			Result:  "Successful",
		},
		TxnDetails: make([]model.TransactionDetails, 0),
	}

	td.TxnDetails = append(td.TxnDetails, res...)

	return s.RenderJSON(req, &td, http.StatusOK)
}

// @Summary Get transaction details by Transcation Comment
// @Description Retrieves the details of a transaction based on its comment.
// @Tags         Account
// @ID get-by-comment
// @Accept json
// @Produce json
// @Param Comment query string true "Comment to identify the transaction"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-by-comment [get]
func (s *Server) APIGetTxnByComment(req *ensweb.Request) *ensweb.Result {
	comment := s.GetQuerry(req, "Comment")
	res, err := s.c.GetTxnDetailsByComment(comment)
	if err != nil {
		if err.Error() == "no records found" {
			s.log.Info("There are no records present for the comment " + comment)
			td := model.TxnDetails{
				BasicResponse: model.BasicResponse{
					Status:  true,
					Message: "no records present for the comment : " + comment,
				},
				TxnDetails: make([]model.TransactionDetails, 0),
			}
			return s.RenderJSON(req, &td, http.StatusOK)
		}
		s.log.Error("err", err)
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
			},
			TxnDetails: make([]model.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
		},
		TxnDetails: make([]model.TransactionDetails, 0),
	}

	for i := range res {
		td.TxnDetails = append(td.TxnDetails, res[i])
	}

	return s.RenderJSON(req, &td, http.StatusOK)
}

// @Summary Get count of incoming and outgoing txns of the DID ins a node
// @Description Get count of incoming and outgoing txns of the DID ins a node.
// @ID get-txn-details-by-node
// @Tags         Account
// @Accept json
// @Produce json
// @Success 200 {object} model.TxnCountForDID
// @Router /api/get-by-node [get]
func (s *Server) APIGetTxnByNode(req *ensweb.Request) *ensweb.Result {
	dir, ok := s.validateAccess(req)
	if !ok {
		return s.BasicResponse(req, false, "Unathuriozed access", nil)
	}
	if s.cfg.EnableAuth {
		// always expect client token to present
		token, ok := req.ClientToken.Model.(*setup.BearerToken)
		if ok {
			dir = token.DID
		}
	}
	Result := model.TxnCountForDID{
		BasicResponse: model.BasicResponse{
			Status: false,
		}}
	DIDInNode := s.c.GetDIDs(dir)
	for _, d := range DIDInNode {
		txnCount, err := s.c.GetCountofTxn(d.DID)
		if err != nil {
			Result.BasicResponse.Message = err.Error()
			return s.RenderJSON(req, &Result, http.StatusOK)
		}
		Result.BasicResponse.Status = true
		Result.TxnCount = append(Result.TxnCount, txnCount)
	}
	return s.RenderJSON(req, &Result, http.StatusOK)
}

// @Summary Get FT transaction details by DID
// @Description Retrieves the details of a FT transaction based on DID, role (sender or receiver) and date range (on and after start date and before end date).
// @ID get-ft-txn-by-did
// @Tags FT
// @Accept json
// @Produce json
// @Param DID query string true "DID of sender/receiver"
// @Param Role query string false "Filter by role as sender or receiver"
// @Param StartDate query string false "Start date of the date range (format: YYYY-MM-DD)"
// @Param EndDate query string false "End date of the date range (format: YYYY-MM-DD)"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-ft-txn-by-did [get]
func (s *Server) APIGetFTTxnByDID(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "DID")
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9-]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	role := s.GetQuerry(req, "Role")
	startDate := s.GetQuerry(req, "StartDate")
	endDate := s.GetQuerry(req, "EndDate")

	results, err := s.c.GetFTTransactionsByDID(did, role, startDate, endDate)
	if err != nil {
		s.log.Info("Error fetching FT transaction details. " + err.Error())
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
				Result:  "No data found",
			},
			TxnDetails: make([]model.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	ftTransactionDetails := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved FT Txn Details",
			Result:  "Successful",
		},
		TxnDetails: make([]model.TransactionDetails, 0),
	}

	ftTransactionDetails.TxnDetails = append(ftTransactionDetails.TxnDetails, results...)

	return s.RenderJSON(req, &ftTransactionDetails, http.StatusOK)
}

// @Summary Get FT transaction status
// @Description Retrieves the status of FT transactions grouped by transaction ID. Optionally filter by specific transaction ID.
// @ID get-ft-transaction-status
// @Tags FT
// @Accept json
// @Produce json
// @Param DID query string true "DID of sender/receiver"
// @Param transactionID query string false "Optional: Filter by specific transaction ID"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-ft-transaction-status [get]
func (s *Server) APIGetFTTransactionStatus(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "DID")
	transactionID := s.GetQuerry(req, "transactionID") // Optional parameter

	// Debug logging to see what's failing
	s.log.Info("DID validation debug",
		"did", did,
		"length", len(did),
		"starts_with_bafybmi", strings.HasPrefix(did, "bafybmi"),
		"is_alphanumeric", regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(did))

	// More permissive regex to handle edge cases
	is_valid := regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_valid {
		s.log.Error("Invalid DID", "did", did, "length", len(did), "starts_with_bafybmi", strings.HasPrefix(did, "bafybmi"), "is_valid", is_valid)
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}

	// Get transaction status from core
	status, err := s.c.GetFTTransactionStatus(did, transactionID)
	if err != nil {
		s.log.Error("Error fetching FT transaction status", "err", err)
		return s.BasicResponse(req, false, "Failed to fetch transaction status: "+err.Error(), nil)
	}

	return s.BasicResponse(req, true, "Retrieved FT transaction status successfully", status)
}

// @Summary Coordinated rollback for failed transactions
// @Description Handles coordinated rollback requests from senders when confirmation timeout occurs
// @ID coordinated-rollback
// @Tags FT
// @Accept json
// @Produce json
// @Param transaction_id body string true "Transaction ID to rollback"
// @Param token_count body int true "Number of tokens to rollback"
// @Param token_type body int true "Type of tokens (FT or RBT)"
// @Param rollback_type body string true "Type of rollback (coordinated_rollback)"
// @Param reason body string true "Reason for rollback"
// @Success 200 {object} model.BasicResponse
// @Router /api/coordinated-rollback [post]
func (s *Server) APICoordinatedRollback(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	rollbackReq := struct {
		TransactionID string `json:"transaction_id"`
		TokenCount    int    `json:"token_count"`
		TokenType     int    `json:"token_type"`
		RollbackType  string `json:"rollback_type"`
		Reason        string `json:"reason"`
	}{}

	// Access data from the request
	if req.Data != nil {
		if txID, ok := req.Data["transaction_id"].(string); ok {
			rollbackReq.TransactionID = txID
		}
		if tokenCount, ok := req.Data["token_count"].(float64); ok {
			rollbackReq.TokenCount = int(tokenCount)
		}
		if tokenType, ok := req.Data["token_type"].(float64); ok {
			rollbackReq.TokenType = int(tokenType)
		}
		if rollbackType, ok := req.Data["rollback_type"].(string); ok {
			rollbackReq.RollbackType = rollbackType
		}
		if reason, ok := req.Data["reason"].(string); ok {
			rollbackReq.Reason = reason
		}
	}

	// Validate request
	if rollbackReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	if rollbackReq.TokenCount <= 0 {
		return s.BasicResponse(req, false, "Token count must be positive", nil)
	}

	if rollbackReq.RollbackType != "coordinated_rollback" {
		return s.BasicResponse(req, false, "Invalid rollback type", nil)
	}

	s.log.Info("Received coordinated rollback request",
		"transaction_id", rollbackReq.TransactionID,
		"token_count", rollbackReq.TokenCount,
		"token_type", rollbackReq.TokenType,
		"reason", rollbackReq.Reason)

	// Perform the rollback
	err := s.c.CoordinatedRollback(rollbackReq.TransactionID, rollbackReq.TokenType)
	if err != nil {
		s.log.Error("Failed to perform coordinated rollback",
			"transaction_id", rollbackReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Rollback failed: "+err.Error(), nil)
	}

	s.log.Info("Coordinated rollback completed successfully",
		"transaction_id", rollbackReq.TransactionID,
		"token_count", rollbackReq.TokenCount)

	return s.BasicResponse(req, true, "Coordinated rollback completed successfully", nil)
}

// @Summary Send token transfer confirmation
// @Description Allows receivers to send confirmation to senders that tokens have been successfully processed
// @ID send-token-confirmation
// @Tags FT
// @Accept json
// @Produce json
// @Param transaction_id body string true "Transaction ID to confirm"
// @Param receiver_did body string true "DID of the receiver"
// @Param token_count body int true "Number of tokens confirmed"
// @Param token_type body int true "Type of tokens (FT or RBT)"
// @Param status body string true "Status of confirmation (success/failure)"
// @Success 200 {object} model.BasicResponse
// @Router /api/send-token-confirmation [post]
func (s *Server) APISendTokenConfirmation(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	confirmationReq := struct {
		TransactionID string `json:"transaction_id"`
		ReceiverDID   string `json:"receiver_did"`
		TokenCount    int    `json:"token_count"`
		TokenType     int    `json:"token_type"`
		Status        string `json:"status"`
	}{}

	// Access data from the request
	if req.Data != nil {
		if txID, ok := req.Data["transaction_id"].(string); ok {
			confirmationReq.TransactionID = txID
		}
		if receiverDID, ok := req.Data["receiver_did"].(string); ok {
			confirmationReq.ReceiverDID = receiverDID
		}
		if tokenCount, ok := req.Data["token_count"].(float64); ok {
			confirmationReq.TokenCount = int(tokenCount)
		}
		if tokenType, ok := req.Data["token_type"].(float64); ok {
			confirmationReq.TokenType = int(tokenType)
		}
		if status, ok := req.Data["status"].(string); ok {
			confirmationReq.Status = status
		}
	}

	// Validate request
	if confirmationReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	if confirmationReq.ReceiverDID == "" {
		return s.BasicResponse(req, false, "Receiver DID is required", nil)
	}

	if confirmationReq.TokenCount <= 0 {
		return s.BasicResponse(req, false, "Token count must be positive", nil)
	}

	if confirmationReq.Status != "success" && confirmationReq.Status != "failure" {
		return s.BasicResponse(req, false, "Status must be 'success' or 'failure'", nil)
	}

	s.log.Info("Received token confirmation request",
		"transaction_id", confirmationReq.TransactionID,
		"receiver_did", confirmationReq.ReceiverDID,
		"token_count", confirmationReq.TokenCount,
		"token_type", confirmationReq.TokenType,
		"status", confirmationReq.Status)

	// Send confirmation signal to the sender
	err := s.c.SignalConfirmation(confirmationReq.TransactionID)
	if err != nil {
		s.log.Error("Failed to send confirmation signal",
			"transaction_id", confirmationReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Failed to send confirmation: "+err.Error(), nil)
	}

	s.log.Info("Token confirmation sent successfully",
		"transaction_id", confirmationReq.TransactionID,
		"receiver_did", confirmationReq.ReceiverDID,
		"status", confirmationReq.Status)

	return s.BasicResponse(req, true, "Token confirmation sent successfully", nil)
}

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
		RequesterDID  string `json:"requester_did"`  // For remote recovery tracking
		RemoteRequest bool   `json:"remote_request"` // Flag for remote recovery
		Reason        string `json:"reason"`         // Reason for recovery
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

	// Check if this is a remote recovery request
	if recoveryReq.RemoteRequest {
		s.log.Info("Received remote token recovery request",
			"sender_did", recoveryReq.SenderDID,
			"transaction_id", recoveryReq.TransactionID,
			"requester_did", recoveryReq.RequesterDID,
			"reason", recoveryReq.Reason)
	} else {
		s.log.Info("Received token recovery request",
			"sender_did", recoveryReq.SenderDID,
			"transaction_id", recoveryReq.TransactionID)
	}

	// Perform token recovery
	var recoveryResult *core.TokenRecoveryResult
	
	if recoveryReq.RemoteRequest {
		// Handle remote recovery request
		recoveryResult, err = s.c.HandleRemoteRecoveryRequest(
			recoveryReq.SenderDID,
			recoveryReq.TransactionID,
			recoveryReq.RequesterDID,
			recoveryReq.Reason)
	} else {
		// Handle normal recovery request
		recoveryResult, err = s.c.RecoverLostTokens(recoveryReq.SenderDID, recoveryReq.TransactionID)
	}
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
		"reason", remoteReq.Reason,
		"target_node_url", remoteReq.TargetNodeURL)

	// Initiate the remote recovery
	var recoveryResult *core.TokenRecoveryResult
	
	// Use HTTP method if target URL is provided
	if remoteReq.TargetNodeURL != "" {
		s.log.Info("Using HTTP method for remote recovery",
			"target_url", remoteReq.TargetNodeURL)
		recoveryResult, err = s.c.RemoteRecoverTokensHTTP(&remoteReq, remoteReq.TargetNodeURL)
	} else {
		s.log.Info("Using peer connection method for remote recovery")
		recoveryResult, err = s.c.RemoteRecoverTokens(&remoteReq)
	}
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

// @Summary Verify token ownership for recovery
// @Description Allows senders to verify if receiver has tokens for recovery purposes
// @ID verify-token-ownership
// @Tags FT
// @Accept json
// @Produce json
// @Param transaction_id body string true "Transaction ID to verify"
// @Param expected_amount body int true "Expected amount of tokens"
// @Param check_type body string true "Type of check (recovery_verification)"
// @Success 200 {object} model.BasicResponse
// @Router /api/verify-token-ownership [post]
func (s *Server) APIVerifyTokenOwnership(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	verifyReq := struct {
		TransactionID  string `json:"transaction_id"`
		ExpectedAmount int    `json:"expected_amount"`
		CheckType      string `json:"check_type"`
	}{}

	// Access data from the request
	if req.Data != nil {
		if txID, ok := req.Data["transaction_id"].(string); ok {
			verifyReq.TransactionID = txID
		}
		if expectedAmount, ok := req.Data["expected_amount"].(float64); ok {
			verifyReq.ExpectedAmount = int(expectedAmount)
		}
		if checkType, ok := req.Data["check_type"].(string); ok {
			verifyReq.CheckType = checkType
		}
	}

	// Validate request
	if verifyReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	if verifyReq.ExpectedAmount <= 0 {
		return s.BasicResponse(req, false, "Expected amount must be positive", nil)
	}

	if verifyReq.CheckType != "recovery_verification" {
		return s.BasicResponse(req, false, "Invalid check type", nil)
	}

	s.log.Info("Received token ownership verification request",
		"transaction_id", verifyReq.TransactionID,
		"expected_amount", verifyReq.ExpectedAmount)

	// Check if this node has the tokens for the given transaction
	hasTokens, err := s.c.VerifyLocalTokenOwnership(verifyReq.TransactionID, verifyReq.ExpectedAmount)
	if err != nil {
		s.log.Error("Failed to verify local token ownership",
			"transaction_id", verifyReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Verification failed: "+err.Error(), nil)
	}

	if hasTokens {
		s.log.Info("Token ownership verified - tokens exist",
			"transaction_id", verifyReq.TransactionID,
			"expected_amount", verifyReq.ExpectedAmount)
		return s.BasicResponse(req, true, "Tokens found", nil)
	} else {
		s.log.Info("Token ownership verified - tokens not found",
			"transaction_id", verifyReq.TransactionID,
			"expected_amount", verifyReq.ExpectedAmount)
		return s.BasicResponse(req, false, "Tokens not found", nil)
	}
}

// @Summary Check token transfer status for recovery
// @Description Allows senders to check if tokens were transferred from receiver for recovery purposes
// @ID check-token-transfer-status
// @Tags FT
// @Accept json
// @Produce json
// @Param transaction_id body string true "Transaction ID to check"
// @Param check_type body string true "Type of check (transfer_verification)"
// @Success 200 {object} model.BasicResponse
// @Router /api/check-token-transfer-status [post]
func (s *Server) APICheckTokenTransferStatus(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	checkReq := struct {
		TransactionID string `json:"transaction_id"`
		CheckType     string `json:"check_type"`
	}{}

	// Access data from the request
	if req.Data != nil {
		if txID, ok := req.Data["transaction_id"].(string); ok {
			checkReq.TransactionID = txID
		}
		if checkType, ok := req.Data["check_type"].(string); ok {
			checkReq.CheckType = checkType
		}
	}

	// Validate request
	if checkReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	if checkReq.CheckType != "transfer_verification" {
		return s.BasicResponse(req, false, "Invalid check type", nil)
	}

	s.log.Info("Received token transfer status check request",
		"transaction_id", checkReq.TransactionID)

	// Check if tokens were transferred from this node for the given transaction
	transferred, err := s.c.CheckLocalTokenTransferStatus(checkReq.TransactionID)
	if err != nil {
		s.log.Error("Failed to check local token transfer status",
			"transaction_id", checkReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Check failed: "+err.Error(), nil)
	}

	if transferred {
		s.log.Info("Token transfer status checked - tokens were transferred",
			"transaction_id", checkReq.TransactionID)
		return s.BasicResponse(req, true, "Tokens were transferred", nil)
	} else {
		s.log.Info("Token transfer status checked - tokens were not transferred",
			"transaction_id", checkReq.TransactionID)
		return s.BasicResponse(req, false, "Tokens were not transferred", nil)
	}
}

// @Summary Verify token existence in storage
// @Description Allows senders to verify if a specific token exists in receiver's storage
// @ID verify-token-existence
// @Tags FT
// @Accept json
// @Produce json
// @Param token_id body string true "Token ID to verify"
// @Param transaction_id body string true "Transaction ID associated with the token"
// @Param verify_type body string true "Type of verification (token_existence)"
// @Success 200 {object} model.BasicResponse
// @Router /api/verify-token-existence [post]
func (s *Server) APIVerifyTokenExistence(req *ensweb.Request) *ensweb.Result {
	// Parse request data from ensweb request
	verifyReq := struct {
		TokenID       string `json:"token_id"`
		TransactionID string `json:"transaction_id"`
		VerifyType    string `json:"verify_type"`
	}{}

	// Access data from the request
	if req.Data != nil {
		if tokenID, ok := req.Data["token_id"].(string); ok {
			verifyReq.TokenID = tokenID
		}
		if transactionID, ok := req.Data["transaction_id"].(string); ok {
			verifyReq.TransactionID = transactionID
		}
		if verifyType, ok := req.Data["verify_type"].(string); ok {
			verifyReq.VerifyType = verifyType
		}
	}

	// Validate request
	if verifyReq.TokenID == "" {
		return s.BasicResponse(req, false, "Token ID is required", nil)
	}

	if verifyReq.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}

	if verifyReq.VerifyType != "token_existence" {
		return s.BasicResponse(req, false, "Invalid verify type", nil)
	}

	s.log.Info("Received token existence verification request",
		"token_id", verifyReq.TokenID,
		"transaction_id", verifyReq.TransactionID)

	// Check if this token exists in local storage
	tokenExists, err := s.c.VerifyLocalTokenExistence(verifyReq.TokenID, verifyReq.TransactionID)
	if err != nil {
		s.log.Error("Failed to verify local token existence",
			"token_id", verifyReq.TokenID,
			"transaction_id", verifyReq.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, "Verification failed: "+err.Error(), nil)
	}

	if tokenExists {
		s.log.Info("Token existence verified - token found",
			"token_id", verifyReq.TokenID,
			"transaction_id", verifyReq.TransactionID)
		return s.BasicResponse(req, true, "Token found", nil)
	} else {
		s.log.Info("Token existence verified - token not found",
			"token_id", verifyReq.TokenID,
			"transaction_id", verifyReq.TransactionID)
		return s.BasicResponse(req, false, "Token not found", nil)
	}
}
