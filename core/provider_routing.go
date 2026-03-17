package core

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

type PinStatusReq struct {
	TokenHash string `json:"token_hash"`
}

type PinStatusRes struct {
	Status    bool   `json:"status"`
	TokenHash string `json:"token_hash"`
	DID       string `json:"did"`
	FuncID    int    `json:"funcid"`
	Role      int    `json:"role"`
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
	record, err := c.ipfsProviderStore.GetProviderByCID(reqObj.TokenHash)
	if err != nil {
		c.log.Error("error getting provider info for token hash ", reqObj.TokenHash, "error", err)
		return c.l.RenderJSON(req, &res, http.StatusOK)
	}
	res.Status = true
	res.TokenHash = record.CID
	res.DID = record.DID
	res.Role = record.Role

	return c.l.RenderJSON(req, &res, http.StatusOK)
}
