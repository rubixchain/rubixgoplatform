package server

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIMineRBT(req *ensweb.Request) *ensweb.Result {
	fmt.Println("APIMineRBT function called in server module")
	var miningReq model.MiningRequest
	// var payload map[string]string
	err := s.ParseJSON(req, &miningReq)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	_, did, ok := util.ParseAddress(miningReq.MinerDid)
	if !ok {
		return s.BasicResponse(req, false, "Miner Did is missing in request", nil)
	}
	s.log.Debug("did from the querry is:", did)
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(did)
	if !strings.HasPrefix(did, "bafybmi") || len(did) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.InitiateMineRBT(req.ID, &miningReq)
	return s.didResponse(req, req.ID)

}