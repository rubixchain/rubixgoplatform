package server

import (
	"strconv"
	"time"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type syncTransactionChainRequest struct {
	DID                   string   `json:"did"`
	TokenIDs              []string `json:"token_ids"`
	ExcludeTransactionIDs []string `json:"exclude_transaction_ids,omitempty"`
}

// @Summary Initiates a transaction
// @Description Submits a transfer request (RBT, FT, NFT or smart contract) and runs it through quorum consensus. Returns a request ID that must be passed to the signature API (POST /rubix/v1/signature) to sign and complete the transaction.
// @ID txInit
// @Tags Transactions
// @Accept json
// @Produce json
// @Param   input body models.TransactionRequest true "transaction"
// @Success 200 {object} models.BasicResponse
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
// @Description  Returns the transaction details for the given transaction ID.
// @Tags         Transactions
// @ID           txQuery
// @Accept       json
// @Produce      json
// @Param 		 tx_id path string true "Transaction ID"
// @Success      200  {object}  models.BasicResponse
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
// @Description  Returns all transactions stored on this node.
// @Tags         Transactions
// @ID           getAllTx
// @Accept       json
// @Produce      json
// @Success      200  {object}  models.BasicResponse
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
// @Tags         Transactions
// @ID           syncTxChain
// @Accept       json
// @Produce      json
// @Param        input body syncTransactionChainRequest true "sync request"
// @Success      200  {object}  models.BasicResponse
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

// APIGetTransactionsByDID godoc
// @Summary      Get Transactions by DID
// @Description  Returns transactions for the given DID, filtered by token type (rbt, nft, ft, smartContract).
// @Description  Optional query parameters narrow results further:
// @Description    - latest: return only the N most recent transactions sorted by epoch descending (e.g. latest=5)
// @Description    - start_date / end_date: filter by created_at date range, inclusive, format YYYY-MM-DD
// @Description    - start_epoch / end_epoch: filter by epoch range, inclusive integer values
// @Tags         Transactions
// @ID           getTxnsByDID
// @Accept       json
// @Produce      json
// @Param        did         path   string  true   "DID"
// @Param        token_type  path   string  true   "Token Type (rbt, nft, ft, smartContract)"
// @Param        latest      query  int     false  "Return only the N most recent transactions sorted by epoch descending"
// @Param        start_date  query  string  false  "Start date for created_at filter (YYYY-MM-DD, inclusive)"
// @Param        end_date    query  string  false  "End date for created_at filter (YYYY-MM-DD, inclusive)"
// @Param        start_epoch query  int     false  "Start epoch for epoch range filter (inclusive)"
// @Param        end_epoch   query  int     false  "End epoch for epoch range filter (inclusive)"
// @Success      200  {object}  models.BasicResponse
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

	filter, errMsg := parseTxQueryFilter(req, s)
	if errMsg != "" {
		return s.BasicResponse(req, false, errMsg, nil)
	}

	txInfo, err := s.c.GetTransactionsByDIDAndTokenType(did, tokenType, filter)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	return s.BasicResponse(req, true, "", txInfo)
}

// parseTxQueryFilter reads optional query parameters from the request and returns a TxQueryFilter.
// Returns a non-empty error message string if any parameter value is malformed.
func parseTxQueryFilter(req *ensweb.Request, s *Server) (models.TxQueryFilter, string) {
	var filter models.TxQueryFilter

	if v := s.GetQuery(req, "latest"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return filter, "latest must be a positive integer"
		}
		filter.Latest = n
	}

	const dateLayout = "2006-01-02"
	if v := s.GetQuery(req, "start_date"); v != "" {
		t, err := time.Parse(dateLayout, v)
		if err != nil {
			return filter, "start_date must be in YYYY-MM-DD format"
		}
		filter.StartDate = t.UTC()
	}
	if v := s.GetQuery(req, "end_date"); v != "" {
		t, err := time.Parse(dateLayout, v)
		if err != nil {
			return filter, "end_date must be in YYYY-MM-DD format"
		}
		// include the full end day up to 23:59:59.999999999 UTC
		filter.EndDate = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
	}

	if v := s.GetQuery(req, "start_epoch"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return filter, "start_epoch must be an integer"
		}
		filter.StartEpoch = &n
	}
	if v := s.GetQuery(req, "end_epoch"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return filter, "end_epoch must be an integer"
		}
		filter.EndEpoch = &n
	}

	return filter, ""
}
