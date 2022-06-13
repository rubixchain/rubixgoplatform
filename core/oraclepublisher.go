package core

import (
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

const (
	APIGetTokenToMine  string = "/api/oracle/getTokenToMine"
	APIGetCurrentLevel string = "/api/oracle/getCurrentLevel"
	APIGet             string = "/api/oracle/get"
	APIGetQuorum       string = "/api/oracle/getQuorum"
	APIUpdates         string = "/api/oracle/updates"
	APISyncQuorum      string = "/api/oracle/syncQuorum"
	APISyncDatatable   string = "/api/oracle/syncDataTable"
)

type OracleRequest struct {
	Message string `json:"message"`
}

type OracleResponse struct {
	model.BasicResponse
}

func (c *Core) OracleSetup() {
	c.l.AddRoute(APIGetTokenToMine, "GET", c.GetTokenToMine)
	c.l.AddRoute(APIGetCurrentLevel, "GET", c.GetCurrentLevel)
	c.l.AddRoute(APIGet, "GET", c.Get)
	c.l.AddRoute(APIGetQuorum, "GET", c.GetQuorum)
	c.l.AddRoute(APIUpdates, "GET", c.Updates)
	c.l.AddRoute(APISyncQuorum, "GET", c.Sync)
	c.l.AddRoute(APISyncDatatable, "GET", c.Sync)
}

func (c *Core) Sync(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg interface{}
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) GetTokenToMine(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg []model.TokenID
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) GetCurrentLevel(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg model.TokenID
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) Get(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg []model.NodeID
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) GetQuorum(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg []string
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) Updates(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = false
	resp.Message = "Value not added"
	var msg model.BasicResponse
	c.l.ParseJSON(req, &msg)
	c.oracleLock.Lock()
	if len(c.param) < ResponsesCount && c.oracleFlag == true {
		c.param = append(c.param, msg)
		resp.Status = true
		resp.Message = "Value accepted"
	}
	c.oracleLock.Unlock()
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}
