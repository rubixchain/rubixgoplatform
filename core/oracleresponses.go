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
		//respArray stores the responses in the structure
		//var respArray []model.TokenID
		dict := make(map[model.TokenID]int)
		var maxOccur model.TokenID
		maxOccur = resp[0].(model.TokenID)
		var maxCount = 0
		for _, obj := range resp {
			response := obj.(model.TokenID)
			//respArray = append(respArray, response)
			dict[response] = dict[response] + 1
			if dict[response] > maxCount {
				maxCount = dict[response]
				maxOccur = response
			}
		}

		fmt.Println(dict)
		fmt.Println("Max occuring element is", maxOccur, "Times: ", maxCount)

	case "/getTokenToMine":

		// resp = append(resp, append([]model.TokenID(nil), model.TokenID{Level: 4, Token: 3}))
		// resp = append(resp, append([]model.TokenID(nil), model.TokenID{Level: 4, Token: 3}))
		// resp = append(resp, append([]model.TokenID(nil), model.TokenID{Level: 4, Token: 3}))
		// resp = append(resp, append([]model.TokenID(nil), model.TokenID{Level: 4, Token: 3}))

		//var respArray [][]model.TokenID
		dict := make(map[model.TokenID]int)

		maxOccur := resp[0].([]model.TokenID)[0]
		var maxCount = 0
		for _, obj := range resp {
			response := obj.([]model.TokenID)
			fmt.Println(response[0])
			//respArray = append(respArray, response)
			dict[response[0]] = dict[response[0]] + 1
			if dict[response[0]] > maxCount {
				maxCount = dict[response[0]]
				maxOccur = response[0]
			}
		}
		maxOccurFormatted := append([]model.TokenID(nil), maxOccur)
		fmt.Println(dict)
		fmt.Println("Max occuring element is", maxOccurFormatted, "Times: ", maxCount)

	case "/updateQuorum", "/assigncredits", "/updatemine", "/add":
		var respArray []model.BasicResponse
		for i := 0; i < len(resp); i++ {
			response := resp[i].(model.BasicResponse)
			respArray = append(respArray, response)
		}
		fmt.Println("Printing responses", respArray)

	case "/getQuorum":
		var respArray [][]string
		for _, obj := range resp {
			response := obj.([]string)
			respArray = append(respArray, response)
		}
		fmt.Println("Printing responses", respArray)

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
		fmt.Println("Maximum occuring array is ", maxOccur, "times", maxCount)

	}
}
