package command

import (
	"fmt"
	"os"
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
		for _, v := range mt { // Iterate directly over the slice
			if c1 {
				str = str + ","
			}
			c1 = true
			decodedValue := block.DecodeNestedStructure("", v)
			str, err = tcMarshal(str, decodedValue)
			if err != nil {
				return "", err
			}
		}
		str = str + "]"

	case []interface{}:
		str = str + "["
		c1 := false
		for _, v := range mt { // Iterate directly over the slice
			if c1 {
				str = str + ","
			}
			c1 = true
			// Recursively decode each element in the slice
			decodedValue := block.DecodeNestedStructure("", v)
			str, err = tcMarshal(str, decodedValue)
			if err != nil {
				return "", err
			}
		}
		str = str + "]"
	case map[string]interface{}: // Handle map[string]interface{}
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true

			decodedKey, exists := block.KeyMap[k]
			if !exists {
				decodedKey = k
			}
			str = str + "\"" + decodedKey + "\":"

			// Recursively decode and marshal the value
			decodedValue := block.DecodeNestedStructure(decodedKey, v)
			str, err = tcMarshal(str, decodedValue)
			if err != nil {
				return "", err
			}
		}
		str = str + "}"
	case map[interface{}]interface{}: // Handle map[interface{}]interface{}
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true

			// Convert interface{} key to string
			keyStr := fmt.Sprintf("%v", k)
			decodedKey, exists := block.KeyMap[keyStr]
			if !exists {
				decodedKey = keyStr
			}
			str = str + "\"" + decodedKey + "\":"

			// Recursively decode and marshal the value
			decodedValue := block.DecodeNestedStructure(decodedKey, v)
			str, err = tcMarshal(str, decodedValue)
			if err != nil {
				return "", err
			}
		}
		str = str + "}"
	case map[string]string: // Handle map[string]string (no decoding needed)
		str = str + "{"
		c1 := false
		for k, v := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "\"" + k + "\":\"" + v + "\""
		}
		str = str + "}"

	case []string: // Handle slice of strings (no decoding needed)
		str = str + "["
		c1 := false
		for _, s := range mt {
			if c1 {
				str = str + ","
			}
			c1 = true
			str = str + "\"" + s + "\""
		}
		str = str + "]"

	case []byte: // Handle byte slices
		str = str + "\"" + util.HexToStr(mt) + "\""

	case string:
		str = str + "\"" + mt + "\""

	case uint64, int: // Handle integers
		str = str + fmt.Sprintf("%d", mt)

	case float64: // Handle floating-point numbers
		str = str + fmt.Sprintf("%.5f", mt)

	case nil: // Handle nil values
		str = str + "null"

	default: // Handle unsupported types
		return "", fmt.Errorf("invalid type %T", mt)
	}
	return str, nil
}

// dumpTokenChain dumps the token chain by retrieving blocks from the command's client.
// It iterates through the blocks, decodes each block, and appends the decoded block to a list.
// Finally, it marshals the list of decoded blocks into a JSON string and writes it to a file named "dump.json".
// This method logs debug information about the original and decoded blocks, as well as any errors encountered.
func (cmd *Command) dumpTokenChain() {
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
				// Decode the block before adding it to the list
				decodedBlock := decodeBlock(b.GetBlockMap())
				blocks = append(blocks, decodedBlock)
			} else {
				cmd.log.Error("Invalid block")
			}
		}
		blockID = ds.NextBlockID
		if ds.NextBlockID == "" {
			break
		}
	}

	str, err := tcMarshal("", blocks) // Pass nil for keys to use all keys
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

// dumpSmartContractTokenChain dumps the smart contract token chain by retrieving blocks from the smart contract token chain
// and saving them to a JSON file named "dump.json". It iterates through the blocks until there are no more blocks to retrieve.
// The function returns an error if there is any issue with dumping the smart contract token chain.
func (cmd *Command) dumpSmartContractTokenChain() {
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
				decodedBlock := decodeBlock(b.GetBlockMap())
				blocks = append(blocks, decodedBlock)
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

// decodeBlock decodes the block data and returns it as a map[string]interface{}.
func decodeBlock(blockData map[string]interface{}) map[string]interface{} {
	// Directly using DecodeNestedStructure function
	return block.DecodeNestedStructure("", blockData).(map[string]interface{})
}

// findNestedKeyMapping is a helper function to find nested key mappings. It searches for a nested key mapping in the provided item.
// It takes a key string and an item map as input and returns the mapped key and a boolean indicating if the mapping exists.
// The key is split by "-" to handle the KeyMap format.
// It iterates through the nested structure of the item to find the mapping.
// If the key is not found or the nested structure is not a map, it returns an empty string and false.
// Finally, it reconstructs the original key and returns the mapped key and a boolean indicating if the mapping exists.
func findNestedKeyMapping(key string, item map[string]interface{}) (string, bool) {
	parts := strings.Split(key, "-") // Split by "-" for KeyMap format
	current := item
	for _, part := range parts {
		if next, ok := current[part]; ok {
			current, ok = next.(map[string]interface{}) // Move down the nested structure
			if !ok {
				return "", false // Not a map, cannot be further nested
			}
		} else {
			return "", false // Key not found in the nested structure
		}
	}

	joinedKey := strings.Join(parts, "-") // Reconstruct the original key
	mappedKey, exists := block.KeyMap[joinedKey]
	return mappedKey, exists
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
