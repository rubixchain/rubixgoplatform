package server

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Initiates a transaction
// @Description Initiate a transaction
// @ID tx
// @Tags tx
// @Accept json
// @Produce json
// @Param   input body models.TransactionRequest true ""
// @Success 200 {object} model.BasicResponse
// @Router /rubix/v1/tx [post]
func (s *Server) APIInitiateTransaction(req *ensweb.Request) *ensweb.Result {
	var transactionreq models.TransactionRequest
	err := s.ParseJSON(req, &transactionreq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateTransaction(req.ID, &transactionreq)
	return s.didResponse(req, req.ID)
}

// NFT godoc
// @Summary      Get Transactions by ID
// @Description  Get Transactions by ID
// @Tags         tx
// @ID           tx
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/tx/{tx_id} [get]
func (s *Server) APIGetTransactionByID(req *ensweb.Request) *ensweb.Result {
	txID := s.GetRouteVar(req, "tx_id")
	if txID == "" {
		return s.BasicResponse(req, false, "empty transaction id", nil)
	}

	txInfo, err := s.c.GetTransactionByID(txID)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	return s.BasicResponse(req, true, "", txInfo)
}

// NFT godoc
// @Summary      List transactions
// @Description  List transactions
// @Tags         tx
// @ID           tx
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/tx [get]
func (s *Server) APIGetTransactions(req *ensweb.Request) *ensweb.Result {
	transactions, err := s.c.GetAllTransactions()
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	return s.BasicResponse(req, true, "", transactions)
}

