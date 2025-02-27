package server

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func (s *Server) APIMineRBTs(req *ensweb.Request) *ensweb.Result {
	fmt.Println("APIMineRBTs function called in server module")
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

	err = s.c.FindReadyToMineCredits(did)
	if err != nil {
		return s.BasicResponse(req, false, err.Error(), nil)
	}
	return s.BasicResponse(req, true, "successfully mined RBT's for the provided token credits", nil)

}


