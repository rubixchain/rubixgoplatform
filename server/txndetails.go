package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

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
