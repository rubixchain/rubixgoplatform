package core

import (
	"fmt"
	"net/http"

	"github.com/EnsurityTechnologies/ensweb"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

type OracleRequest struct {
	Message string `json:"message"`
}

type OracleResponse struct {
	model.BasicResponse
}

func (c *Core) OracleSetup() {

	c.l.AddRoute(APIPublisherPath, "GET", c.OracleRecevied)
}

// PingRecevied is the handler for ping request
func (c *Core) OracleRecevied(req *ensweb.Request) *ensweb.Result {
	resp := &OracleResponse{
		BasicResponse: model.BasicResponse{
			Status: false,
		},
	}
	resp.Status = true
	//var msg OracleRequest
	var msg []model.TokenID
	c.l.ParseJSON(req, &msg)
	fmt.Println("Inside publisher", msg)
	resp.Message = "Message Sent Back"
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}
