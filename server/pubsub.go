package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APISubscribeExplorer(req *ensweb.Request) *ensweb.Result {
	err := s.c.ExploreSubscribe()
	if err != nil {
		return s.BasicResponse(req, false, "failed to subscribe explorer", nil)
	}
	return s.BasicResponse(req, true, "Explorer subscribed successfully", nil)
}

func (s *Server) APIPublishExplorer(req *ensweb.Request) *ensweb.Result {
	var exp model.ExploreModel
	err := s.ParseJSON(req, &exp)
	if err != nil {
		return s.BasicResponse(req, false, "failed to parse explorer data", nil)
	}
	err = s.c.PublishExplore(exp)
	if err != nil {
		return s.BasicResponse(req, false, "failed to publish explorer", nil)
	}
	return s.BasicResponse(req, true, "Explorer data published successfully", nil)
}
