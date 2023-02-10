package core

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
)

type PinStatusReq struct {
	Token string `json:"token"`
}

type PinStatusRes struct {
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
	err := c.l.ParseJSON(req, &reqObj)
	c.log.Debug("Token", reqObj.Token)
	if err != nil {
		c.log.Error("error parsing incoming request", "error", err)
		var res PinStatusRes
		return c.l.RenderJSON(req, &res, http.StatusOK)
	}
	providerMap, err := c.w.GetProviderDetails(reqObj.Token)
	if err != nil {
		if err.Error() == "record not found" {
			c.log.Debug("Data not found in table")
		} else {
			c.log.Error("Error", err)
		}
	}
	res := PinStatusRes{
		Token:  providerMap.Token,
		DID:    providerMap.DID,
		FuncID: providerMap.FuncID,
		Role:   providerMap.Role,
	}

	return c.l.RenderJSON(req, &res, http.StatusOK)
}
