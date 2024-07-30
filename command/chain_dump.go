package command

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"io/ioutil"
	"encoding/json"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/spf13/cobra"
)

func chainDumpCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "chain-dump",
		Short: "Token-chain and SmartContract-Chain dump related subcommands ",
		Long: "Token-chain and SmartContract-Chain dump related subcommands ",
	}

	cmd.AddCommand(
		tokenChainDumpCmd(cmdCfg),
		smartContractChainDumpCmd(cmdCfg),
		decodeTokenChain(cmdCfg),
	)

	return cmd
}

func tokenChainDumpCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "token",
		Short: "Get the dump of Token chain",
		Long: "Get the dump of Token chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.token == "" {
				cmdCfg.log.Info("token id cannot be empty")
				fmt.Print("Enter Token Id : ")
				_, err := fmt.Scan(&cmdCfg.token)
				if err != nil {
					cmdCfg.log.Error("Failed to get Token ID")
					return nil
				}
			}
			isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.token)
		
			if len(cmdCfg.token) != 46 || !strings.HasPrefix(cmdCfg.token, "Qm") || !isAlphanumeric {
				cmdCfg.log.Error("Invalid token")
				return nil
			}

			blocks := make([]map[string]interface{}, 0)
			blockID := ""
			for {
				ds, err := cmdCfg.c.DumpTokenChain(cmdCfg.token, blockID)
				if err != nil {
					cmdCfg.log.Error("Failed to dump token chain", "err", err)
					return nil
				}
				if !ds.Status {
					cmdCfg.log.Error("Failed to dump token chain", "msg", ds.Message)
					return nil
				}
				for _, blk := range ds.Blocks {
					b := block.InitBlock(blk, nil)
					if b != nil {
						blocks = append(blocks, b.GetBlockMap())
					} else {
						cmdCfg.log.Error("Invalid block")
					}
				}
				blockID = ds.NextBlockID
				if ds.NextBlockID == "" {
					break
				}
			}
			str, err := tcMarshal("", blocks)
			if err != nil {
				cmdCfg.log.Error("Failed to dump token chain", "err", err)
				return nil
			}
			f, err := os.Create("dump.json")
			if err != nil {
				cmdCfg.log.Error("Failed to dump token chain", "err", err)
				return nil
			}
			f.WriteString(str)
			f.Close()
			cmdCfg.log.Info("Token chain dumped successfully!")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.token, "tokenHash", "", "Token Hash")

	return cmd
}

func smartContractChainDumpCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "smart-contract",
		Short: "Get the dump of Smart Contract chain",
		Long: "Get the dump of Smart Contract chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.smartContractToken == "" {
				cmdCfg.log.Info("smart contract token id cannot be empty")
				fmt.Print("Enter SC Token Id : ")
				_, err := fmt.Scan(&cmdCfg.smartContractToken)
				if err != nil {
					cmdCfg.log.Error("Failed to get SC Token ID")
					return nil
				}
			}
			is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.smartContractToken)
		
			if len(cmdCfg.smartContractToken) != 46 || !strings.HasPrefix(cmdCfg.smartContractToken, "Qm") || !is_alphanumeric {
				cmdCfg.log.Error("Invalid smart contract token")
				return nil
			}

			blocks := make([]map[string]interface{}, 0)
			blockID := ""
			for {
				ds, err := cmdCfg.c.DumpSmartContractTokenChain(cmdCfg.smartContractToken, blockID)
				if err != nil {
					cmdCfg.log.Error("Failed to dump smart contract token chain", "err", err)
					return nil
				}
				if !ds.Status {
					cmdCfg.log.Error("Failed to dump smart contract token chain", "msg", ds.Message)
					return nil
				}
				for _, blk := range ds.Blocks {
					b := block.InitBlock(blk, nil)
					if b != nil {
						blocks = append(blocks, b.GetBlockMap())
					} else {
						cmdCfg.log.Error("Invalid block")
					}
				}
				blockID = ds.NextBlockID
				if ds.NextBlockID == "" {
					break
				}
			}
			str, err := tcMarshal("", blocks)
			if err != nil {
				cmdCfg.log.Error("Failed to dump smart contract token chain", "err", err)
				return nil
			}
			f, err := os.Create("dump.json")
			if err != nil {
				cmdCfg.log.Error("Failed to dump smart contract token chain", "err", err)
				return nil
			}
			f.WriteString(str)
			f.Close()
			cmdCfg.log.Info("smart contract Token chain dumped successfully!")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")

	return cmd
}

// decodeTokenChain decodes a JSON file, transforms its data, and writes the transformed data back to a file.
func decodeTokenChain(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "decode-token-chain",
		Short: "Decode Token chain",
		Long: "Decond Token chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Open the input JSON file
			file, err := os.Open("dump.json")
			if err != nil {
				errMsg := fmt.Errorf("error opening file: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}
			defer file.Close()

			// Read the JSON file
			byteValue, err := ioutil.ReadAll(file)
			if err != nil {
				errMsg := fmt.Errorf("error reading file: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}

			// Parse the JSON data
			var data []interface{}
			err = json.Unmarshal(byteValue, &data)
			if err != nil {
				errMsg := fmt.Errorf("error parsing JSON: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
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
				errMsg := fmt.Errorf("error marshaling JSON: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}

			// Write the output to a file
			err = ioutil.WriteFile("output.json", output, 0644)
			if err != nil {
				errMsg := fmt.Errorf("error writing file: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}

			cmdCfg.log.Info("Transformation complete. Check output.json for results.")
			return nil
		},
	}

	return cmd
	
	
}

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