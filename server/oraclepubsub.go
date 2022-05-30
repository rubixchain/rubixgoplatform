package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APISubscribeOracle(req *ensweb.Request) *ensweb.Result {
	err := s.c.OracleSubscribe()
	if err != nil {
		return s.BasicResponse(req, false, "failed to subscribe explorer", nil)
	}
	return s.BasicResponse(req, true, "Oracle subscribed successfully", nil)
}

func (s *Server) APIPublishOracle(req *ensweb.Request) *ensweb.Result {
	var input model.AssignCredits
	err := s.ParseJSON(req, &input)
	if err != nil {
		return s.BasicResponse(req, false, "failed to parse oracle data", nil)
	}
	err = s.c.PublishOracle(input)
	if err != nil {
		return s.BasicResponse(req, false, "failed to publish explorer", nil)
	}
	return s.BasicResponse(req, true, "Oracle data published successfully", nil)
}
