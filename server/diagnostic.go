package server

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APIDumpTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var dr model.TCDumpRequest
	err := s.ParseJSON(req, &dr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	drep := s.c.DumpTokenChain(&dr)
	return s.RenderJSON(req, drep, http.StatusOK)
}

func (s *Server) APIDumpSmartContractTokenChainBlock(req *ensweb.Request) *ensweb.Result {
	var dr model.TCDumpRequest
	err := s.ParseJSON(req, &dr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid input", nil)
	}
	drep := s.c.DumpSmartContractTokenChain(&dr)
	return s.RenderJSON(req, drep, http.StatusOK)
}
