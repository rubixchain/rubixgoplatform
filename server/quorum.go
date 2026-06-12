package server

import (
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APISetupQuorum godoc
// @Summary      Setup quorum
// @Description  Sets up the quorum for the given DID using the supplied passwords.
// @Tags         Quorum
// @Accept       json
// @Produce      json
// @Param        input  body      models.QuorumSetup  true  "Quorum setup request"
// @Success      200    {object}  models.BasicResponse
// @Router       /rubix/v1/quorums/setup [post]
func (s *Server) APISetupQuorum(req *ensweb.Request) *ensweb.Result {
	var qs models.QuorumSetup
	err := s.ParseJSON(req, &qs)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse the input", nil)
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(qs.DID)
	if !strings.HasPrefix(qs.DID, "bafybmi") || len(qs.DID) != 59 || !is_alphanumeric {
		s.log.Error("Invalid DID")
		return s.BasicResponse(req, false, "Invalid DID", nil)
	}
	err = s.c.SetupQuorum(qs.DID, qs.Password, qs.PrivKeyPassword)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to setup quorum, "+err.Error(), nil)
	}
	return s.BasicResponse(req, true, "Quorum setup done successfully", nil)
}
