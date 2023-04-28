package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// @Summary Get transaction details by Transcation ID
// @Description Retrieves the details of a transaction based on its ID.
// @ID get-txn-details-by-id
// @Tags         Account
// @Accept json
// @Produce json
// @Param txnID query string true "The ID of the transaction to retrieve"
// @Success 200 {object} model.BasicResponse
// @Failure 400 {object} model.BasicResponse
// @Failure 404 {object} model.BasicResponse
// @Failure 500 {object} model.BasicResponse
// @Router /api/get-txn-details-by-id [get]
func (s *Server) APIGetTxnByTxnID(req *ensweb.Request) *ensweb.Result {
	txnID := s.GetQuerry(req, "txnID")
	res, err := s.c.GetTxnDetailsByID(txnID)
	if err != nil {
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
// @Description Retrieves the details of a transaction based on dID.
// @ID get-by-did
// @Tags         Account
// @Accept json
// @Produce json
// @Param DID query string true "DID of sender/receiver"
// @Param Role query string false "Filter by role as sender or receiver"
// @Success 200 {object} model.BasicResponse
// @Failure 400 {object} model.BasicResponse
// @Failure 404 {object} model.BasicResponse
// @Failure 500 {object} model.BasicResponse
// @Router /api/get-by-did [get]
func (s *Server) APIGetTxnByDID(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "DID")
	role := s.GetQuerry(req, "Role")
	res, err := s.c.GetTxnDetailsByDID(did, role)
	if err != nil {
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

// @Summary Get transaction details by Transcation Comment
// @Description Retrieves the details of a transaction based on its comment.
// @Tags         Account
// @ID get-by-comment
// @Accept json
// @Produce json
// @Param Comment query string true "Comment to identify the transaction"
// @Success 200 {object} model.BasicResponse
// @Failure 400 {object} model.BasicResponse
// @Failure 404 {object} model.BasicResponse
// @Failure 500 {object} model.BasicResponse
// @Router /api/get-by-comment [get]
func (s *Server) APIGetTxnByComment(req *ensweb.Request) *ensweb.Result {
	comment := s.GetQuerry(req, "Comment")
	res, err := s.c.GetTxnDetailsByComment(comment)
	if err != nil {
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
