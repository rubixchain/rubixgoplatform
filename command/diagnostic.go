package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/util"
)

func tcMarshal(str string, m interface{}) (string, error) {
	var err error
	switch mt := m.(type) {
	case []map[string]interface{}:
		str = str + "["
		c1 := false
		for i := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str, err = tcMarshal(str, mt[i])
			if err != nil {
				return "", err
			}
		}
		str = str + "]"
	case map[string]interface{}:
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "\"" + k + "\":"
			s, ok := v.(string)
			if ok {
				str = str + "\"" + s + "\""
			} else {
				str, err = tcMarshal(str, v)
				if err != nil {
					return "", err
				}
			}
		}
		str = str + "}"
	case map[string]string:
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "\"" + k + "\":"
			str = str + "\"" + v + "\""
		}

		str = str + "}"
	case map[interface{}]interface{}:
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "\"" + k.(string) + "\":"
			str, err = tcMarshal(str, v)
			if err != nil {
				return "", err
			}
		}

		str = str + "}"
	case []string:
		str = str + "["
		c1 := false
		for _, mf := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str, err = tcMarshal(str, mf)
			if err != nil {
				return "", err
			}
		}
		str = str + "]"
	case []byte:
		str = str + "\"" + util.HexToStr(mt) + "\""
	case string:
		str = str + "\"" + mt + "\""
	case []interface{}:
		str = str + "["
		c1 := false
		for _, mf := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			s, ok := mf.(string)
			if ok {
				str = str + "\"" + s + "\""
			} else {
				str, err = tcMarshal(str, mf)
				if err != nil {
					return "", err
				}
			}
		}
		str = str + "]"
	case uint64:
		str = str + fmt.Sprintf("%d", mt)
	case int:
		str = str + fmt.Sprintf("%d", mt)
	case float64:
		// TokenValue (key: "10") is a float value and needs to have a precision of 5
		// in the output dump file
		str = str + fmt.Sprintf("%.5f", mt)
	case interface{}:
		str, err = tcMarshal(str, mt)
		if err != nil {
			return "", err
		}
	case nil:
		str = str + "\"" + "\""
	default:
		return "", fmt.Errorf("invalid type %T", mt)
	}
	return str, nil
}

func (cmd *Command) dumpTokenChain() {
	if cmd.token == "" {
		cmd.log.Info("token id cannot be empty")
		fmt.Print("Enter Token Id : ")
		_, err := fmt.Scan(&cmd.token)
		if err != nil {
			cmd.log.Error("Failed to get Token ID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.token)

	if len(cmd.token) != 46 || !strings.HasPrefix(cmd.token, "Qm") || !isAlphanumeric {
		cmd.log.Error("Invalid token")
		return
	}

	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	for {
		ds, err := cmd.c.DumpTokenChain(cmd.token, blockID)
		if err != nil {
			cmd.log.Error("Failed to dump token chain", "err", err)
			return
		}
		if !ds.Status {
			cmd.log.Error("Failed to dump token chain", "msg", ds.Message)
			return
		}
		for _, blk := range ds.Blocks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				cmd.log.Error("Invalid block")
			}
		}
		blockID = ds.NextBlockID
		if ds.NextBlockID == "" {
			break
		}
	}
	str, err := tcMarshal("", blocks)
	if err != nil {
		cmd.log.Error("Failed to dump token chain", "err", err)
		return
	}
	f, err := os.Create("dump.json")
	if err != nil {
		cmd.log.Error("Failed to dump token chain", "err", err)
		return
	}
	f.WriteString(str)
	f.Close()
	cmd.log.Info("Token chain dumped successfully!")
}

func (cmd *Command) dumpFTTokenchain() {
	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	for {
		ds, err := cmd.c.DumpFTTokenChain(cmd.token, blockID)
		if err != nil {
			cmd.log.Error("Failed to dump token chain", "err", err)
			return
		}
		if !ds.Status {
			cmd.log.Error("Failed to dump token chain", "msg", ds.Message)
			return
		}
		for _, blk := range ds.Blocks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				cmd.log.Error("Invalid block")
			}
		}
		blockID = ds.NextBlockID
		if ds.NextBlockID == "" {
			break
		}
	}
	str, err := tcMarshal("", blocks)
	if err != nil {
		cmd.log.Error("Failed to dump token chain", "err", err)
		return
	}
	f, err := os.Create("dump.json")
	if err != nil {
		cmd.log.Error("Failed to dump token chain", "err", err)
		return
	}
	f.WriteString(str)
	f.Close()
	cmd.log.Info("Token chain dumped successfully!")
}

func (cmd *Command) dumpSmartContractTokenChain() {
	if cmd.smartContractToken == "" {
		cmd.log.Info("smart contract token id cannot be empty")
		fmt.Print("Enter SC Token Id : ")
		_, err := fmt.Scan(&cmd.smartContractToken)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)

	if len(cmd.smartContractToken) != 46 || !strings.HasPrefix(cmd.smartContractToken, "Qm") || !isAlphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}
	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	for {
		ds, err := cmd.c.DumpSmartContractTokenChain(cmd.smartContractToken, blockID)
		if err != nil {
			cmd.log.Error("Failed to dump smart contract token chain", "err", err)
			return
		}
		if !ds.Status {
			cmd.log.Error("Failed to dump smart contract token chain", "msg", ds.Message)
			return
		}
		for _, blk := range ds.Blocks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				cmd.log.Error("Invalid block")
			}
		}
		blockID = ds.NextBlockID
		if ds.NextBlockID == "" {
			break
		}
	}
	str, err := tcMarshal("", blocks)
	if err != nil {
		cmd.log.Error("Failed to dump smart contract token chain", "err", err)
		return
	}
	f, err := os.Create("dump.json")
	if err != nil {
		cmd.log.Error("Failed to dump smart contract token chain", "err", err)
		return
	}
	f.WriteString(str)
	f.Close()
	cmd.log.Info("smart contract Token chain dumped successfully!")
}

func (cmd *Command) dumpNFTTokenChain() {
	if cmd.nft == "" {
		cmd.log.Info("NFT id cannot be empty")
		fmt.Print("Enter NFT Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get NFT Token ID")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.smartContractToken)

	if len(cmd.nft) != 46 || !strings.HasPrefix(cmd.nft, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid nft")
		return
	}
	blocks := make([]map[string]interface{}, 0)
	blockID := ""
	for {
		ds, err := cmd.c.DumpNFTTokenChain(cmd.nft, blockID)
		if err != nil {
			cmd.log.Error("Failed to get nft token chain", "err", err)
			return
		}
		if !ds.Status {
			cmd.log.Error("Failed to get nft token chain", "msg", ds.Message)
			return
		}
		for _, blk := range ds.Blocks {
			b := block.InitBlock(blk, nil)
			if b != nil {
				blocks = append(blocks, b.GetBlockMap())
			} else {
				cmd.log.Error("Invalid block")
			}
		}
		blockID = ds.NextBlockID
		if ds.NextBlockID == "" {
			break
		}
	}
	str, err := tcMarshal("", blocks)
	if err != nil {
		cmd.log.Error("Failed to get nft token chain", "err", err)
		return
	}
	f, err := os.Create("nft.json")
	if err != nil {
		cmd.log.Error("Failed to write nft token chain to file", "err", err)
		return
	}
	f.WriteString(str)
	f.Close()
	cmd.log.Info("NFT Token chain dumped successfully!")
}

// decodeTokenChain decodes a JSON file, transforms its data, and writes the transformed data back to a file.
func (cmd *Command) decodeTokenChain() {
	// Open the input JSON file
	file, err := os.Open("dump.json")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Read the JSON file
	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Parse the JSON data
	var data []interface{}
	err = json.Unmarshal(byteValue, &data)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	// Transform the JSON data
	for i, item := range data {
		flattenedItem := flattenKeys("", item)
		mappedItem := applyKeyMapping(flattenedItem)
		data[i] = mappedItem
	}

	// Convert the transformed data back to JSON
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	// Write the output to a file
	err = ioutil.WriteFile("output.json", output, 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
		return
	}

	fmt.Println("Transformation complete. Check output.json for results.")
}

func (cmd *Command) getTokenBlock() {

}

func (cmd *Command) getSmartContractData() {
	// if latest flag not set then return all data
	// format willbe json object
	/*
		{
			block_no
			block_hash
			smartcontract_hash
		}
	*/

}

func (cmd *Command) removeTokenChainBlock() {
	response, err := cmd.c.RemoveTokenChainBlock(cmd.token, cmd.latest)
	if err != nil {
		cmd.log.Error("Failed to remove token chain", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to remove token chain", "msg", response.Message)
		return
	}
	cmd.log.Info("Token chain removed successfully!")
}

func (cmd *Command) releaseAllLockedTokens() {
	resp, err := cmd.c.ReleaseAllLockedTokens()
	if err != nil {
		cmd.log.Error("Failed to release the locked tokens", "err", err)
		return
	}
	if !resp.Status {
		cmd.log.Error("Failed to release the locked tokens", "msg", resp.Message)
		return
	}
	cmd.log.Info("Locked Tokens released successfully Or No Locked Tokens found to be released")
}
