package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (s *Server) APIPublishContract(request *ensweb.Request) *ensweb.Result {
	var newEvent model.NewContractEvent
	err := s.ParseJSON(request, &newEvent)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}

	go s.c.PublishNewEvent(&newEvent)
	return s.BasicResponse(request, true, "Smart contract published successfully", nil)
}
func (s *Server) APISubscribecontract(request *ensweb.Request) *ensweb.Result {
	var newSubscription model.NewSubscription
	err := s.ParseJSON(request, &newSubscription)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}
	topic := newSubscription.Contract
	s.c.AddWebReq(request)
	go s.c.SubsribeContractSetup(request.ID, topic)
	return s.BasicResponse(request, true, "Smart contract subscribed successfully", nil)
}
