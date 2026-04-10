package server

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// @Summary Initiates a transaction
// @Description Initiate a transaction
// @ID txInit
// @Tags Transactions
// @Accept json
// @Produce json
// @Param   input body models.TransactionRequest true "transaction"
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
// @Tags         Transactions
// @ID           txQuery
// @Accept       json
// @Produce      json
// @Param 		 tx_id path string true "Transaction ID"
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
// @Tags         Transactions
// @ID           getAllTx
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

// APIGetTransactionsByDID godoc
// @Summary      Get Transactions by DID
// @Description  Get Transactions by DID
// @Tags         Transactions
// @ID           getTxnsByDID
// @Accept       json
// @Produce      json
// @Param        did        path   string  true   "DID"
// @Param        token_type path  string  true  "Token Type (rbt, nft, ft, smartContract)"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/tx/{did}/{token_type} [get]
func (s *Server) APIGetTransactionsByDID(req *ensweb.Request) *ensweb.Result {
	did := s.GetRouteVar(req, "did")
	if did == "" {
		return s.BasicResponse(req, false, "empty did", nil)
	}

	tokenType := s.GetRouteVar(req, "token_type")
	if tokenType == "" {
		return s.BasicResponse(req, false, "empty token type", nil)
	}

	txInfo, err := s.c.GetTransactionsByDIDAndTokenType(did, tokenType)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	return s.BasicResponse(req, true, "", txInfo)
}
