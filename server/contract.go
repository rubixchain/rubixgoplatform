package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APIPublishEvent(request *ensweb.Request) *ensweb.Result {
	var newEvent model.NewContractEvent
	err := s.ParseJSON(request, &newEvent)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}

	go s.c.PublishNewEvent(&newEvent)
	return s.BasicResponse(request, true, "", nil)
}
func (s *Server) APISubscribecontract(request *ensweb.Request) *ensweb.Result {
	var newSubcription model.NewSubcription
	err := s.ParseJSON(request, &newSubcription)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}
	topic := newSubcription.Contract
	go s.c.SubsribeContractSetup(topic)
	return s.BasicResponse(request, true, "", nil)
}
