package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (s *Server) APIDeploySmartContract(req *ensweb.Request) *ensweb.Result {
	var deployReq model.DeploySmartContractRequest
	err := s.ParseJSON(req, &deployReq)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	_, did, ok := util.ParseAddress(deployReq.DeployerAddress)
	if !ok {
		return s.BasicResponse(req, false, "Invalid Deployer address", nil)
	}
	if !s.validateDIDAccess(req, did) {
		return s.BasicResponse(req, false, "DID does not have an access", nil)
	}
	s.c.AddWebReq(req)
	go s.c.DeploySmartContractToken(req.ID, &deployReq)
	return s.didResponse(req, req.ID)
}
