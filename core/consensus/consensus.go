package consensus

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/parts"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

func BuildTokenInfos(tokens []models.Token) []models.TokenInfo {
	tokenInfos := make([]TokenInfo, len(tokens))

	for i, t := range tokens {
		tokenInfos[i] = TokenInfo{
			TokenID:               t.TokenID,
			PreviousTransactionID: t.TransactionID,
		}
	}

	return tokenInfos
}

func (c *Core) reqPledgeToken(request *ensweb.Request) *ensweb.Result {
	did := c.l.GetQuery(request, "did")

	var pledgeTokenRequest models.PledgeTokenRequest
	err := c.l.ParseJSON(request, &pledgeTokenRequest)

	crep := model.BasicResponse{Status: false}

	c.log.Debug("Request for pledge tokens", "did", did)

	if err != nil {
		c.log.Error("Failed to parse json request", "err", err)
		crep.Message = "Invalid request body"
		return c.l.RenderJSON(request, &crep, http.StatusBadRequest)
	}

	_, ok := c.qc[did]
	if !ok {
		c.log.Error("Quorum is not setup", "did", did)
		crep.Message = "Quorum is not setup"
		return c.l.RenderJSON(request, &crep, http.StatusNotFound)
	}

	dc := c.pqc[did]

	pledgeTokenDetails, err := parts.CollectRBTTokens(
		dc,
		c.w,
		pledgeTokenRequest.TokensRequired,
		c.testnet,
		c.log,
		c.publishTxn,
	)

	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		crep.Message = "Failed to get tokens"
		return c.l.RenderJSON(request, &crep, http.StatusInternalServerError)
	}

	if len(pledgeTokenDetails) == 0 {
		crep.Message = "No tokens left to pledge"
		return c.l.RenderJSON(request, &crep, http.StatusConflict)
	}

	pledgeTokens := BuildTokenInfos(pledgeTokenDetails)

	pledgeResponse := models.PledgeTokenResponse{
		ReferenceId:  request.ReferenceId,
		PledgeTokens: pledgeTokens,
	}

	return c.l.RenderJSON(request, &pledgeResponse, http.StatusOK)
}
