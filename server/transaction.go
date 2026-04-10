package server

import (
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type syncTransactionChainRequest struct {
	DID                   string   `json:"did"`
	TokenIDs              []string `json:"token_ids"`
	ExcludeTransactionIDs []string `json:"exclude_transaction_ids,omitempty"`
}

// @Summary Initiates a transaction
// @Description Initiate a transaction
// @ID txInit
// @Tags tx
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
// @Tags         tx
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
// @Tags         tx
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

// APISyncTransactionChain godoc
// @Summary      Sync transaction chains for tokens
// @Description  Returns ordered transaction chains for the requested token IDs, optionally excluding specific transaction IDs
// @Tags         tx
// @ID           syncTxChain
// @Accept       json
// @Produce      json
// @Param        input body syncTransactionChainRequest true "sync request"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/sync-transaction-chain [post]
func (s *Server) APISyncTransactionChain(req *ensweb.Request) *ensweb.Result {
	var syncReq syncTransactionChainRequest
	err := s.ParseJSON(req, &syncReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if len(syncReq.TokenIDs) == 0 {
		return s.BasicResponse(req, true, "no token_ids provided", nil)
	}
	data, err := s.c.GetSyncTransactionChainData(syncReq.TokenIDs, syncReq.ExcludeTransactionIDs)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.BasicResponse(req, true, "ok", data)
}

