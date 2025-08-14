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
