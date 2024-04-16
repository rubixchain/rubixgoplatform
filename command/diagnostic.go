package command

import (
	"fmt"
	"os"

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
		str = str + fmt.Sprintf("%f", mt)
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
