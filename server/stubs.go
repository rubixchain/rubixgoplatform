package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APISetupDB handles the legacy setup-db endpoint.
// TODO(phase11-upstream): wire to PostgreSQL config or retire.
func (s *Server) APISetupDB(req *ensweb.Request) *ensweb.Result {
	var sc config.StorageConfig
	if err := s.ParseJSON(req, &sc); err != nil {
		return s.BasicResponse(req, false, "invalid input", nil)
	}
	return s.BasicResponse(req, false, "not implemented in PostgreSQL build", nil)
}

// APIGetTxnByTxnID handles the legacy get-by-txnId endpoint.
// TODO(phase11-upstream): implement via PostgreSQL transaction queries.
func (s *Server) APIGetTxnByTxnID(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, map[string]interface{}{"status": false, "message": "not implemented"}, http.StatusOK)
}

// APIGetTxnByDID handles the legacy get-by-did endpoint.
// TODO(phase11-upstream): implement via PostgreSQL transaction queries.
func (s *Server) APIGetTxnByDID(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, map[string]interface{}{"status": false, "message": "not implemented"}, http.StatusOK)
}

// APIGetTxnByComment handles the legacy get-by-comment endpoint.
// TODO(phase11-upstream): implement via PostgreSQL transaction queries.
func (s *Server) APIGetTxnByComment(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, map[string]interface{}{"status": false, "message": "not implemented"}, http.StatusOK)
}

// APIGetTxnByNode handles the legacy get-by-node endpoint.
// TODO(phase11-upstream): implement via PostgreSQL transaction queries.
func (s *Server) APIGetTxnByNode(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, map[string]interface{}{"status": false, "message": "not implemented"}, http.StatusOK)
}

// RunUnpledgeHandle handles the run-unpledge endpoint.
// TODO(phase11-upstream): implement unpledge logic.
func (s *Server) RunUnpledgeHandle(req *ensweb.Request) *ensweb.Result {
	return s.BasicResponse(req, false, "not implemented", nil)
}

// APIGetFTTxnByDID handles the get-ft-txn-by-did endpoint.
// TODO(phase11-upstream): implement via PostgreSQL FT transaction queries.
func (s *Server) APIGetFTTxnByDID(req *ensweb.Request) *ensweb.Result {
	return s.RenderJSON(req, map[string]interface{}{"status": false, "message": "not implemented"}, http.StatusOK)
}
