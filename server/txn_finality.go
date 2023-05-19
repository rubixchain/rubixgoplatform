package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APIGetPendingTxn(req *ensweb.Request) (res *ensweb.Result) {
	response, err := s.c.GetFinalityPendingTxns()
	if err != nil {
		s.log.Error("Error", err)
		result := model.PendingTxnIds{
			BasicResponse: model.BasicResponse{
				Status:  false,
				Message: "Error Triggered" + err.Error(),
			},
			TxnIds: make([]string, 0),
		}
		return s.RenderJSON(req, &result, http.StatusOK)
	}

	result := model.PendingTxnIds{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "Finality Pending Txns Retrieved",
		},
		TxnIds: make([]string, 0),
	}

	for i := range response {
		result.TxnIds = append(result.TxnIds, response[i])
	}
	return s.RenderJSON(req, &result, http.StatusOK)
}

func (s *Server) APIInitiateTxnFinality(req *ensweb.Request) (res *ensweb.Result) {
	txnID := s.GetQuerry(req, "txnID")
	response := s.c.InitiateRBTTxnFinality(txnID)
	if !response.Status {
		s.log.Error("Error Occured", response.Message)
		return s.RenderJSON(req, &response, http.StatusOK)
	}
	return s.RenderJSON(req, &response, http.StatusOK)
}
