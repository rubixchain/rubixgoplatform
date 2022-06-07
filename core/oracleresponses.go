package core

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (c *Core) ValidateResponses(input model.Input, resp []interface{}) {
	switch input.Function {
	case "/getCurrentLevel":
		//respArray stores the responses in the structure
		var respArray []model.TokenID
		dict := make(map[model.TokenID]int)
		// resp = append(resp, model.TokenID{Level: 4, Token: 3})
		// resp = append(resp, model.TokenID{Level: 4, Token: 3})
		// resp = append(resp, model.TokenID{Level: 4, Token: 3})
		// resp = append(resp, model.TokenID{Level: 4, Token: 3})
		// resp = append(resp, model.TokenID{Level: 4, Token: 3})

		var maxOccur model.TokenID
		maxOccur = resp[0].(model.TokenID)
		var maxCount = 0
		for _, obj := range resp {
			response := obj.(model.TokenID)
			respArray = append(respArray, response)
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

		var respArray [][]model.TokenID
		dict := make(map[model.TokenID]int)

		maxOccur := resp[0].([]model.TokenID)[0]
		var maxCount = 0
		for _, obj := range resp {
			response := obj.([]model.TokenID)
			fmt.Println(response[0])
			respArray = append(respArray, response)
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
		var respArray [][]model.NodeID
		for _, obj := range resp {
			response := obj.([]model.NodeID)
			respArray = append(respArray, response)
		}
		fmt.Println("Printing responses", respArray)
	}
}
