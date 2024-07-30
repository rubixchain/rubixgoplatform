package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/spf13/cobra"
)

func generateSmartContractTokenCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a Smart Contract Token",
		Long:  "Generate a Smart Contract Token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Info("DID cannot be empty")
				fmt.Print("Enter DID : ")
				_, err := fmt.Scan(&cmdCfg.did)
				if err != nil {
					cmdCfg.log.Error("Failed to get DID")
					return nil
				}
			}
			is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.did)
			if !strings.HasPrefix(cmdCfg.did, "bafybmi") || len(cmdCfg.did) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid DID")
				return nil
			}
			if cmdCfg.binaryCodePath == "" {
				cmdCfg.log.Error("Please provide Binary code file")
				return nil
			}
			if cmdCfg.rawCodePath == "" {
				cmdCfg.log.Error("Please provide Raw code file")
				return nil
			}
			if cmdCfg.schemaFilePath == "" {
				cmdCfg.log.Error("Please provide Schema file")
				return nil
			}

			smartContractTokenRequest := core.GenerateSmartContractRequest{
				BinaryCode: cmdCfg.binaryCodePath,
				RawCode:    cmdCfg.rawCodePath,
				SchemaCode: cmdCfg.schemaFilePath,
				DID:        cmdCfg.did,
			}

			request := client.SmartContractRequest{
				BinaryCode: smartContractTokenRequest.BinaryCode,
				RawCode:    smartContractTokenRequest.RawCode,
				SchemaCode: smartContractTokenRequest.SchemaCode,
				DID:        smartContractTokenRequest.DID,
			}

			basicResponse, err := cmdCfg.c.GenerateSmartContractToken(&request)
			if err != nil {
				cmdCfg.log.Error("Failed to generate smart contract token", "err", err)
				return nil
			}
			if !basicResponse.Status {
				cmdCfg.log.Error("Failed to generate smart contract token", "err", err)
				return nil
			}
			cmdCfg.log.Info("Smart contract token generated successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.binaryCodePath, "binCode", "", "WASM binary path")
	cmd.Flags().StringVar(&cmdCfg.rawCodePath, "rawCode", "", "Smart Contract code path (usually lib.rs)")
	cmd.Flags().StringVar(&cmdCfg.schemaFilePath, "schemaFile", "", "Schema file path")

	return cmd
}

func fetchSmartContractCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch a Smart Contract Token",
		Long:  "Fetch a Smart Contract Token",
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

			smartContractTokenRequest := core.FetchSmartContractRequest{
				SmartContractToken: cmdCfg.smartContractToken,
			}

			request := client.FetchSmartContractRequest{
				SmartContractToken: smartContractTokenRequest.SmartContractToken,
			}

			basicResponse, err := cmdCfg.c.FetchSmartContract(&request)
			if err != nil {
				cmdCfg.log.Error("Failed to fetch smart contract token", "err", err)
				return nil
			}
			if !basicResponse.Status {
				cmdCfg.log.Error("Failed to fetch smart contract token", "err", err)
				return nil
			}

			cmdCfg.log.Info("Smart contract token fetched successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")

	return cmd
}

func publishContract(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a Smart Contract Token",
		Long:  "Publish a Smart Contract Token",
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
			is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.did)
			if !strings.HasPrefix(cmdCfg.did, "bafybmi") || len(cmdCfg.did) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid DID")
				return nil
			}
			if cmdCfg.publishType < 1 || cmdCfg.publishType > 2 {
				cmdCfg.log.Error("Invalid publish type")
				return nil
			}

			basicResponse, err := cmdCfg.c.PublishNewEvent(
				cmdCfg.smartContractToken, 
				cmdCfg.did, 
				cmdCfg.publishType, 
				cmdCfg.newContractBlock,
			)
		
			if err != nil {
				cmdCfg.log.Error("Failed to publish new event", "err", err)
				return nil
			}
			if !basicResponse.Status {
				cmdCfg.log.Error("Failed to publish new event", "msg", basicResponse.Message)
				return nil
			}
			message, status := signatureResponse(cmdCfg, basicResponse)
		
			if !status {
				cmdCfg.log.Error("Failed to publish new event, " + message)
				return nil
			}

			cmdCfg.log.Info("New event published successfully")
			return nil
		},
	}

	cmd.Flags().IntVar(&cmdCfg.publishType, "pubType", 0, "Smart contract event publishing type(Deploy & Execute)")
	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")
	cmd.Flags().StringVar(&cmdCfg.newContractBlock, "sctBlockHash", "", "Contract block hash")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}

func subscribeContract(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "subscribe",
		Short: "Subscribe to a Smart Contract",
		Long: "Subscribe to a Smart Contract",
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

			basicResponse, err := cmdCfg.c.SubscribeContract(cmdCfg.smartContractToken)

			if err != nil {
				cmdCfg.log.Error("Failed to subscribe contract", "err", err)
				return nil
			}
			if !basicResponse.Status {
				cmdCfg.log.Error("Failed to subscribe contract", "msg", basicResponse.Message)
				return nil
			}
			message, status := signatureResponse(cmdCfg, basicResponse)

			if !status {
				cmdCfg.log.Error("Failed to subscribe contract, " + message)
				return nil
			}
			cmdCfg.log.Info("New event subscribed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")

	return cmd
}

func deploySmartcontract(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "deploy",
		Short: "Deploy a Smart Contract",
		Long: "Deploy a Smart Contract",
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
			is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.deployerAddr)
			if !strings.HasPrefix(cmdCfg.deployerAddr, "bafybmi") || len(cmdCfg.deployerAddr) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid deployer DID")
				return nil
			}
			if cmdCfg.rbtAmount < 0.00001 {
				cmdCfg.log.Error("Invalid RBT amount. Minimum RBT amount should be 0.00001")
				return nil
			}
			if cmdCfg.transType < 1 || cmdCfg.transType > 2 {
				cmdCfg.log.Error("Invalid trans type")
				return nil
			}

			deployRequest := model.DeploySmartContractRequest{
				SmartContractToken: cmdCfg.smartContractToken,
				DeployerAddress:    cmdCfg.deployerAddr,
				RBTAmount:          cmdCfg.rbtAmount,
				QuorumType:         cmdCfg.transType,
				Comment:            cmdCfg.transComment,
			}
			response, err := cmdCfg.c.DeploySmartContract(&deployRequest)
			if err != nil {
				cmdCfg.log.Error("Failed to deploy Smart contract, Token ", cmdCfg.smartContractToken, "err", err)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, response)
			if !status {
				cmdCfg.log.Error("Failed to deploy Smart contract, Token ", cmdCfg.smartContractToken, "msg", msg)
				return nil
			}
			cmdCfg.log.Info(msg)
			cmdCfg.log.Info("Smart Contract Deployed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.deployerAddr, "deployerAddr", "", "Smart contract Deployer Address")
	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")
	cmd.Flags().Float64Var(&cmdCfg.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	cmd.Flags().StringVar(&cmdCfg.transComment, "transComment", "", "Transaction comment")
	cmd.Flags().IntVar(&cmdCfg.transType, "transType", 2, "Transaction type")

	return cmd
}

func executeSmartcontract(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "execute",
		Short: "Execute a Smart Contract",
		Long: "Execute a Smart Contract",
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
		
			is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.executorAddr)
			if !strings.HasPrefix(cmdCfg.executorAddr, "bafybmi") || len(cmdCfg.executorAddr) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid executer DID")
				return nil
			}
			if cmdCfg.transType < 1 || cmdCfg.transType > 2 {
				cmdCfg.log.Error("Invalid trans type")
				return nil
			}
			if cmdCfg.smartContractData == "" {
				fmt.Print("Enter Data to be executed : ")
				_, err := fmt.Scan(&cmdCfg.smartContractData)
				if err != nil {
					cmdCfg.log.Error("Failed to get data")
					return nil
				}
			}
			
			executorRequest := model.ExecuteSmartContractRequest{
				SmartContractToken: cmdCfg.smartContractToken,
				ExecutorAddress:    cmdCfg.executorAddr,
				QuorumType:         cmdCfg.transType,
				Comment:            cmdCfg.transComment,
				SmartContractData:  cmdCfg.smartContractData,
			}
			response, err := cmdCfg.c.ExecuteSmartContract(&executorRequest)
			if err != nil {
				cmdCfg.log.Error("Failed to execute Smart contract, Token ", cmdCfg.smartContractToken, "err", err)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, response)
			if !status {
				cmdCfg.log.Error("Failed to execute Smart contract, Token ", cmdCfg.smartContractToken, "msg", msg)
				return nil
			}
			cmdCfg.log.Info(msg)
			cmdCfg.log.Info("Smart Contract executed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.executorAddr, "executorAddr", "", "Smart contract Executor Address")
	cmd.Flags().StringVar(&cmdCfg.smartContractToken, "sct", "", "Smart contract token hash")
	cmd.Flags().Float64Var(&cmdCfg.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	cmd.Flags().StringVar(&cmdCfg.transComment, "transComment", "", "Transaction comment")
	cmd.Flags().IntVar(&cmdCfg.transType, "transType", 2, "Transaction type")

	return cmd
}
