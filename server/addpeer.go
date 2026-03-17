package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type DIDPeerMapTemp struct {
	DID    string
	PeerID string
}

// APIAddPeerDetails godoc
// @Summary     Add Peer
// @Description This API allows the user to add peer details manually
// @Tags        Account
// @Accept      json
// @Produce     json
// @Param       input body DIDPeerMapTemp true "Peer Details"
// @Success     200 {object} model.BasicResponse
// @Router      /api/add-peer-details [post]
func (s *Server) APIAddPeerDetails(req *ensweb.Request) *ensweb.Result {
	var pd DIDPeerMapTemp
	var peerDetails models.DID
	err := s.ParseJSON(req, &pd)
	if err != nil {
		return s.BasicResponse(req, false, "invalid input request", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(pd.PeerID)
	if !strings.HasPrefix(pd.PeerID, "12D3KooW") || len(pd.PeerID) != 52 || !is_alphanumeric {
		return s.BasicResponse(req, false, "Invalid Peer ID", nil)
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(pd.DID)
	if !strings.HasPrefix(pd.DID, "bafybmi") || len(pd.DID) != 59 || !is_alphanumeric {
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	peerDetails.DID = pd.DID
	peerDetails.PeerID = pd.PeerID
	peerDetails.Local = false
	peerDetails.AlgoID = int64(models.GetDidAlgoType(constants.DidAlgo_SECP256K1))

	err = s.c.AddPeerDetails(peerDetails)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add peers in DB, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Peers added successfully", nil)
}
