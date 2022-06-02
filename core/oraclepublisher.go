package core

import (
	"fmt"
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
}

func (c *Core) GetTokenToMine(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	var msg []model.TokenID
	c.l.ParseJSON(req, &msg)
	fmt.Println(msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) GetCurrentLevel(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	var msg model.TokenID
	c.l.ParseJSON(req, &msg)
	fmt.Println(msg)
	c.param = append(c.param, msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) Get(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	var msg []model.NodeID
	c.l.ParseJSON(req, &msg)
	fmt.Println(msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) GetQuorum(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	var msg []string
	c.l.ParseJSON(req, &msg)
	fmt.Println(msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}

func (c *Core) Updates(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	var msg model.BasicResponse
	c.l.ParseJSON(req, &msg)
	fmt.Println(msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}
