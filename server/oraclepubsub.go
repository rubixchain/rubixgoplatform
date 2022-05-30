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
	var exp model.ExploreModel
	err := s.ParseJSON(req, &exp)
	if err != nil {
		return s.BasicResponse(req, false, "failed to parse explorer data", nil)
	}
	err = s.c.PublishOracle(exp)
	if err != nil {
		return s.BasicResponse(req, false, "failed to publish explorer", nil)
	}
	return s.BasicResponse(req, true, "Oracle data published successfully", nil)
}
