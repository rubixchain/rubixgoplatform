package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
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
				TxnDetails: make([]wallet.TransactionDetails, 0),
			}
			return s.RenderJSON(req, &td, http.StatusOK)
		}
		s.log.Error("err", err)
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
			},
			TxnDetails: make([]wallet.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
		},
		TxnDetails: make([]wallet.TransactionDetails, 0),
	}
	td.TxnDetails = append(td.TxnDetails, res)

	return s.RenderJSON(req, &td, http.StatusOK)
}

// @Summary Get transaction details by dID
// @Description Retrieves the details of a transaction based on dID and date range.
// @ID get-by-did
// @Tags Account
// @Accept json
// @Produce json
// @Param DID query string true "DID of sender/receiver"
// @Param Role query string false "Filter by role as sender or receiver"
// @Param StartDate query string false "Start date of the date range (format: YYYY-MM-DD"
// @Param EndDate query string false "End date of the date range (format: YYYY-MM-DD)"
// @Success 200 {object} model.BasicResponse
// @Router /api/get-by-did [get]
func (s *Server) APIGetTxnByDID(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "DID")
	role := s.GetQuerry(req, "Role")
	startDate := s.GetQuerry(req, "StartDate")
	endDate := s.GetQuerry(req, "EndDate")

	if (startDate != "" || endDate != "") && role != "" {
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: "Either use Date range or Role for filter",
				Result:  "",
			},
			TxnDetails: make([]wallet.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}

	res, err := s.c.GetTxnDetailsByDID(did, role, startDate, endDate)
	if err != nil {
		if err.Error() == "no records found" {
			s.log.Info("There are no records present for this DID " + did)
			td := model.TxnDetails{
				BasicResponse: model.BasicResponse{
					Status:  true,
					Message: "no records present for this DID : " + did,
					Result:  "No data found",
				},
				TxnDetails: make([]wallet.TransactionDetails, 0),
			}
			return s.RenderJSON(req, &td, http.StatusOK)
		}
		//s.log.Error("err", err)
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
				Result:  "No data found",
			},
			TxnDetails: make([]wallet.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
			Result:  "Successful",
		},
		TxnDetails: make([]wallet.TransactionDetails, 0),
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
				TxnDetails: make([]wallet.TransactionDetails, 0),
			}
			return s.RenderJSON(req, &td, http.StatusOK)
		}
		s.log.Error("err", err)
		td := model.TxnDetails{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: err.Error(),
			},
			TxnDetails: make([]wallet.TransactionDetails, 0),
		}
		return s.RenderJSON(req, &td, http.StatusOK)
	}
	td := model.TxnDetails{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Retrieved Txn Details",
		},
		TxnDetails: make([]wallet.TransactionDetails, 0),
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
