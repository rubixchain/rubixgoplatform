package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APIPublishEvent(req *ensweb.Request) *ensweb.Result {
	var ne model.NewContractEvent
	err := s.ParseJSON(req, &ne)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}

	go s.c.PublishNewEvent(&ne)
	return s.BasicResponse(req, true, "", nil)
}
func (s *Server) APISubscribecontract(req *ensweb.Request) *ensweb.Result {
	var ns model.NewSubcription
	err := s.ParseJSON(req, &ns)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	topic := ns.Contract
	go s.c.SubsribeContractSetup(topic)
	return s.BasicResponse(req, true, "", nil)
}
