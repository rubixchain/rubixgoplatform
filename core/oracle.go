package core

import (
	"fmt"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) oracle(input model.Input) {

	port := map[string]string{"did": "9090", "adv": "9595"}

	cfg := config.Config{
		ServerAddress: "localhost",
		ServerPort:    port[input.Server],
	}
	cl, err := ensweb.NewClient(&cfg, c.log)
	if err != nil {
		return
	}
	switch input.Function {
	case "/getCurrentLevel":
		req, err := cl.JSONRequest("GET", input.Function, input.Input)
		if err != nil {
			fmt.Println("Error during sending request", err)
			fmt.Println(err)
			return
		}
		resp, err := cl.Do(req)
		if err != nil {
			return
		}
		var response model.TokenID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)

	case "/getTokenToMine":
		req, err := cl.JSONRequest("GET", input.Function, input.Input)
		if err != nil {
			return
		}
		resp, err := cl.Do(req)
		if err != nil {
			return
		}
		var response []model.TokenID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/add":
		req, err := cl.JSONRequest("POST", input.Function, input.Input)
		if err != nil {
			return
		}
		resp, err := cl.Do(req)
		if err != nil {
			return
		}
		var response model.BasicResponse
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	case "/get":
		req, err := cl.JSONRequest("GET", input.Function, input.Input)
		if err != nil {
			return
		}
		resp, err := cl.Do(req)
		if err != nil {
			return
		}
		var response []model.NodeID
		err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
		if err != nil {
			fmt.Println("Invalid response")
			return
		}
		fmt.Println(response)
	}
	// case "/assigncredits":
	// 	req, err := cl.JSONRequest("POST", input.Function, input.Input)
	// 	if err != nil {
	// 		return
	// 	}
	// 	resp, err := cl.Do(req)
	// 	if err != nil {
	// 		return
	// 	}
	// 	var response model.BasicResponse
	// 	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	// 	if err != nil {
	// 		fmt.Println("Invalid response")
	// 		return
	// 	}
	// 	fmt.Println(response)
	// case "/updateQuorum":
	// 	req, err := cl.JSONRequest("POST", input.Function, input.UpdateQuorumInput)
	// 	if err != nil {
	// 		return
	// 	}
	// 	resp, err := cl.Do(req)
	// 	if err != nil {
	// 		return
	// 	}
	// 	var response model.BasicResponse
	// 	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	// 	if err != nil {
	// 		fmt.Println("Invalid response")
	// 		return
	// 	}
	// 	fmt.Println(response)
	// case "/getQuorum":
	// 	req, err := cl.JSONRequest("POST", input.Function, input.GetQuorumInput)
	// 	if err != nil {
	// 		return
	// 	}
	// 	resp, err := cl.Do(req)
	// 	if err != nil {
	// 		return
	// 	}
	// 	var response []string
	// 	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	// 	if err != nil {
	// 		fmt.Println("Invalid response")
	// 		return
	// 	}
	// 	fmt.Println(response)
	// case "/updatemine":
	// 	req, err := cl.JSONRequest("POST", input.Function, input.UpdateMineInput)
	// 	if err != nil {
	// 		return
	// 	}
	// 	resp, err := cl.Do(req)
	// 	if err != nil {
	// 		return
	// 	}
	// 	var response model.BasicResponse
	// 	err = jsonutil.DecodeJSONFromReader(resp.Body, &response)
	// 	if err != nil {
	// 		fmt.Println("Invalid response")
	// 		return
	// 	}
	// 	fmt.Println(response)
	// }

}
