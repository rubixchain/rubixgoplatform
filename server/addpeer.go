package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type DIDPeerMapTemp struct {
	DID     string
	DIDType int
	PeerID  string
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
	var peer_detail wallet.DIDPeerMap
	err := s.ParseJSON(req, &pd)
	if err != nil {
		return s.BasicResponse(req, false, "invalid input request", nil)
	}
	if pd.DIDType < 0 || pd.DIDType > 4 {
		return s.BasicResponse(req, false, "Invalid DID Type", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(pd.PeerID)
	if !strings.HasPrefix(pd.PeerID, "12D3KooW") || len(pd.PeerID) != 52 || !is_alphanumeric {
		return s.BasicResponse(req, false, "Invalid Peer ID", nil)
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(pd.DID)
	if !strings.HasPrefix(pd.DID, "bafybmi") || len(pd.DID) != 59 || !is_alphanumeric {
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	peer_detail.DID = pd.DID
	peer_detail.PeerID = pd.PeerID
	peer_detail.DIDType = &pd.DIDType
	err = s.c.AddPeerDetails(peer_detail)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to add peers in DB, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Peers added successfully", nil)
}
