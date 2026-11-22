package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIStartRecovery godoc
// @Summary      Start account recovery
// @Description  Signs a recovery request for the DID, puts it on the Recovery NFT and deploys the request's ephemeral NFT, which opens the commit window. Runs through the signature flow: answer "Password needed" with POST /rubix/v1/signature. The final result is the request transaction ID. A DID can start one recovery every 48 hours.
// @Tags         Recovery
// @Accept       json
// @Produce      json
// @Param        input  body      models.StartRecoveryRequest  true  "DID to recover"
// @Success      200    {object}  models.BasicResponse
// @Router       /rubix/v1/recovery/start [post]
func (s *Server) APIStartRecovery(req *ensweb.Request) *ensweb.Result {
	var rr models.StartRecoveryRequest
	if err := s.ParseJSON(req, &rr); err != nil {
		return s.BasicResponse(req, false, "Failed to parse the input", nil)
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(rr.DID)
	if !strings.HasPrefix(rr.DID, "bafybmi") || len(rr.DID) != 59 || !isAlphanumeric {
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	s.c.AddWebReq(req)
	go s.c.StartRecovery(req.ID, rr.DID)
	return s.didResponse(req, req.ID)
}

// APIGetRecoverySession godoc
// @Summary      Get recovery session status
// @Description  Returns this node's view of a recovery: its role, state and window deadlines, the messages on the ephemeral NFT, the commitment audit (fullnode) and the chains received (user).
// @Tags         Recovery
// @Produce      json
// @Param        request_txn_id  path      string  true  "Request transaction ID returned by start recovery"
// @Success      200             {object}  types.RecoverySessionResponse
// @Router       /rubix/v1/recovery/sessions/{request_txn_id} [get]
func (s *Server) APIGetRecoverySession(req *ensweb.Request) *ensweb.Result {
	requestID := s.GetRouteVar(req, "request_txn_id")
	if requestID == "" {
		return s.BasicResponse(req, false, "request_txn_id is required", nil)
	}
	status, err := s.c.GetRecoverySessionStatus(requestID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get recovery session, "+err.Error(), nil)
	}
	if status == nil {
		return s.BasicResponse(req, false, "No recovery session for this request on this node", nil)
	}
	return s.BasicResponse(req, true, "Recovery session retrieved", status)
}

// APIRecoveryOutcome godoc
// @Summary      Publish the recovery outcome
// @Description  Publishes the user's outcome of a recovery on its ephemeral NFT, after the sync window closes: a mismatch report listing every quorum or fullnode commitment that did not match the chains delivered, or a completion notice if all matched. Runs the verification first if it has not run. Refused if an outcome is already published. Runs through the signature flow: answer "Password needed" with POST /rubix/v1/signature. The final result is the outcome's transaction ID.
// @Tags         Recovery
// @Accept       json
// @Produce      json
// @Param        input  body      models.RecoveryOutcomeRequest  true  "Request transaction ID returned by start recovery"
// @Success      200    {object}  models.BasicResponse
// @Router       /rubix/v1/recovery/outcome [post]
func (s *Server) APIRecoveryOutcome(req *ensweb.Request) *ensweb.Result {
	var rr models.RecoveryOutcomeRequest
	if err := s.ParseJSON(req, &rr); err != nil {
		return s.BasicResponse(req, false, "Failed to parse the input", nil)
	}
	if rr.RequestTxnID == "" {
		return s.BasicResponse(req, false, "request_txn_id is required", nil)
	}
	s.c.AddWebReq(req)
	go s.c.PublishRecoveryOutcome(req.ID, rr.RequestTxnID)
	return s.didResponse(req, req.ID)
}
