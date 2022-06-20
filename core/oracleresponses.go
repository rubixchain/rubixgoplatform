package core

import (
	"fmt"

	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/ensweb"
	"github.com/EnsurityTechnologies/helper/jsonutil"
	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) ValidateResponses(input model.Input, resp []interface{}) {
	switch input.Function {
	case "/syncdataTable", "/syncQuorum":
		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur interface{}
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj
			}

		}
		fmt.Println("Maximum occuring response is ", maxOccur, "Times: ", maxCount)
		port := map[string]string{"did": "9090", "adv": "9595"}
		funcName := map[string]string{"/syncdataTable": "/postDatatable", "/syncQuorum": "/postQuorum"}
		cfg := config.Config{
			ServerAddress: "localhost",
			ServerPort:    port[input.Server],
		}

		cl, err := ensweb.NewClient(&cfg, c.log)
		if err != nil {
			return
		}
		req, err := cl.JSONRequest("POST", funcName[input.Function], maxOccur)
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
		fmt.Println("Response received from oracle:", response)
	case "/getCurrentLevel":
		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur model.TokenID
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj.(model.TokenID)
			}
		}
		fmt.Println("Maximum occuring response is ", maxOccur, "Times:", maxCount)

	case "/getTokenToMine":
		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur []model.TokenID
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj.([]model.TokenID)
			}

		}
		fmt.Println("Maximum occuring response is ", maxOccur, "Times:", maxCount)

	case "/updateQuorum", "/assigncredits", "/updatemine", "/add":
		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur model.BasicResponse
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj.(model.BasicResponse)
			}

		}
		fmt.Println("Maximum occuring response is ", maxOccur, "Times:", maxCount)

	case "/getQuorum":
		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur []string
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj.([]string)
			}

		}
		fmt.Println("Most occuring response is", maxOccur, "Times:", maxCount)

	case "/get":

		dict := make(map[string]int)
		var maxCount = 0
		var maxOccur []model.NodeID
		for _, obj := range resp {
			str := fmt.Sprintf("%v", obj)
			dict[str] = dict[str] + 1
			if dict[str] > maxCount {
				maxCount = dict[str]
				maxOccur = obj.([]model.NodeID)
			}

		}
		fmt.Println("Maximum occuring array is ", maxOccur, "Times:", maxCount)

	}
}
