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

type syncTransactionInfoFromFullnodeRequest struct {
	TokenIDs []string `json:"token_ids"`
}

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

// APISyncTransactionInfoFromFullnode godoc
// @Summary      Sync transaction info for tokens from fullnode tables
// @Description  Returns ordered transaction info for the requested token IDs from the complete fullnode history
// @Tags         tx
// @ID           syncTransactionInfoFromFullnode
// @Accept       json
// @Produce      json
// @Param        input body syncTransactionInfoFromFullnodeRequest true "sync request"
// @Success      200  {object}  model.BasicResponse
// @Router       /rubix/v1/fullnode/sync-token-chain [post]
func (s *Server) APISyncTransactionInfoFromFullnode(req *ensweb.Request) *ensweb.Result {
	var syncReq syncTransactionInfoFromFullnodeRequest
	err := s.ParseJSON(req, &syncReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	if len(syncReq.TokenIDs) == 0 {
		return s.BasicResponse(req, true, "no token_ids provided", nil)
	}
	// Guard: cap batch size to prevent resource abuse on this public endpoint
	if len(syncReq.TokenIDs) > 50 {
		return s.BasicResponse(req, false, "max 50 token IDs per request", nil)
	}
	data, err := s.c.GetTransactionInfoFromFullnode(syncReq.TokenIDs)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.BasicResponse(req, true, "ok", data)
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
