package core

import (
	"fmt"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) oracle(input model.Input, peerID peer.ID) {
	fmt.Println("Sender's peerID is", peerID)
	port := map[string]string{"did": "9090", "adv": "9595"}
	var MethodType string
	if input.Input == nil {
		MethodType = "GET"
	} else {
		MethodType = "POST"
	}

	cfg := config.Config{
		ServerAddress: "localhost",
		ServerPort:    port[input.Server],
	}

	cl, err := ensweb.NewClient(&cfg, c.log)
	if err != nil {
		return
	}

	req, err := cl.JSONRequest(MethodType, input.Function, input.Input)
	if err != nil {
		fmt.Println("Error during sending request", err)
		fmt.Println(err)
		return
	}
	resp, err := cl.Do(req)
	if err != nil {
		return
	}

	switch input.Function {
	case "/updateQuorum", "/assigncredits", "/updatemine", "/add":
		var response model.BasicResponse
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/getQuorum":
		var response []string
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/get":
		var response []model.NodeID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/getCurrentLevel":
		var response model.TokenID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/getTokenToMine":
		var response []model.TokenID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	}
}
