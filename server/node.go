package server

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIPeerID godoc
// @Summary      Get peer ID
// @Description  Returns the libp2p peer ID of this node.
// @Tags         Peer
// @Produce      json
// @Success      200  {object}  models.BasicResponse
// @Router       /api/get-peer-id [get]
func (s *Server) APIPeerID(req *ensweb.Request) *ensweb.Result {
	return s.BasicResponse(req, true, s.c.GetPeerID(), nil)
}
