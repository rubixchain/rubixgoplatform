// APIUpgradeTokens is a method that handles the upgrade tokens API request.
// It takes a *ensweb.Request as input and returns a *ensweb.Result.
// It parses the JSON input, upgrades tokens server-side, and returns the response.
package server

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ShowAccount godoc
// @Summary      Upgrade Tokens
// @Description  It will upgrade tokens server side
// @Tags         Basic
// @ID 			upgradetokens
// @Accept       json
// @Produce      json
// @Param 		input body RBTTransferRequestSwaggoInput true "Intitate RBT transfer"
// @Success      200  {object}  model.BasicResponse
// @Router       /api/upgrade-tokens [post]
func (s *Server) APIUpgradeTokens(req *ensweb.Request) *ensweb.Result {
	var upgradeRequest core.UpgradeRequest
	err := s.ParseJSON(req, &upgradeRequest)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to parse input", nil)
	}
	basicResponse, err := s.c.UpgradeTokens(&upgradeRequest)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to upgrade tokens", nil)
	}

	fmt.Println("Upgrading tokens server side")

	return s.BasicResponse(req, basicResponse.Status, basicResponse.Message, basicResponse.Result)

}
