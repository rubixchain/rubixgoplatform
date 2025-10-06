package server

import (
	"net/http"
	"regexp"
	"strings"

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
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
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
