package server

import (
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

type NewContractEventSwaggoInput struct {
	Contract          string `json:"contract"`
	Did               string `json:"did"`
	ContractBlockHash string `json:"contract_block_hash"`
}

// PublishContract godoc
// @Summary      Publish Smart Contract
// @Description  This API endpoint publishes a smart contract.
// @Tags         Smart Contract
// @Accept       json
// @Produce      json
// @Param 		 input body NewContractEventSwaggoInput true "Publish input contract"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/publish-contract [post]
func (s *Server) APIPublishContract(request *ensweb.Request) *ensweb.Result {
	var newEvent model.NewContractEvent
	err := s.ParseJSON(request, &newEvent)
	if err != nil {
		return s.BasicResponse(request, false, "Failed to parse input", nil)
	}

	go s.c.PublishNewEvent(&newEvent)
	return s.BasicResponse(request, true, "Smart contract published successfully", nil)
}

type NewSubscriptionSwaggoInput struct {
	Contract string `json:"contract"`
}

// SubscribeContract godoc
// @Summary      Subscribe to Smart Contract
// @Description  This API endpoint allows subscribing to a smart contract.
// @Tags         Smart Contract
// @Accept       json
// @Produce      json
// @Param        input body NewSubscriptionSwaggoInput true "Subscribe to input contract"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/subscribe-contract [post]
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
