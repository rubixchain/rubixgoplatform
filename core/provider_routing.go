package core

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type PinStatusReq struct {
	Token string `json:"token"`
}

type PinStatusRes struct {
	Status bool   `json:"status"`
	Token  string `json:"token"`
	DID    string `json:"did"`
	FuncID int    `json:"funcid"`
	Role   int    `json:"role"`
}

func (c *Core) PinService() {
	c.l.AddRoute(APIDhtProviderCheck, "POST", c.checkProviderStatus)
}

// add logic for checijng the pin of supplied token hash
// return true if pin exist, false if not, reason for pin if true
func (c *Core) checkProviderStatus(req *ensweb.Request) *ensweb.Result {
	var reqObj PinStatusReq
	res := PinStatusRes{}
	err := c.l.ParseJSON(req, &reqObj)
	if err != nil {
		c.log.Error("error parsing incoming request", "error", err)
		return c.l.RenderJSON(req, &res, http.StatusOK)
	}
	providerMap, err := c.w.GetProviderDetails(reqObj.Token)
	if err != nil {
		return c.l.RenderJSON(req, &res, http.StatusOK)
	}
	res.Status = true
	res.Token = providerMap.Token
	res.DID = providerMap.DID
	res.FuncID = providerMap.FuncID
	res.Role = providerMap.Role

	return c.l.RenderJSON(req, &res, http.StatusOK)
}
