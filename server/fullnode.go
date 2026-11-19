package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIRegisterFullnodeDID godoc
// @Summary      Register fullnode recovery DID
// @Description  Registers the local DID a fullnode uses to publish recovery messages. The password is kept in memory only, so register again after a restart.
// @Tags         Fullnode
// @Accept       json
// @Produce      json
// @Param        input  body      models.FullnodeDIDSetup  true  "Fullnode recovery DID"
// @Success      200    {object}  models.BasicResponse
// @Router       /rubix/v1/fullnode/register_did [post]
func (s *Server) APIRegisterFullnodeDID(req *ensweb.Request) *ensweb.Result {
	var rs models.FullnodeDIDSetup
	if err := s.ParseJSON(req, &rs); err != nil {
		return s.BasicResponse(req, false, "Failed to parse the input", nil)
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(rs.DID)
	if !strings.HasPrefix(rs.DID, "bafybmi") || len(rs.DID) != 59 || !isAlphanumeric {
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	if err := s.c.RegisterFullnodeDID(rs.DID, rs.PrivKeyPassword); err != nil {
		return s.BasicResponse(req, false, "Failed to register fullnode recovery DID, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Fullnode recovery DID registered", nil)
}
