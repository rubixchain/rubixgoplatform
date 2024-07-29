package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/spf13/cobra"
)

func transferRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Transfer RBT tokens",
		Long:  "Transfer RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.senderAddr == "" {
				cmdCfg.log.Info("Sender address cannot be empty")
				fmt.Print("Enter Sender DID : ")
				_, err := fmt.Scan(&cmdCfg.senderAddr)
				if err != nil {
					cmdCfg.log.Error("Failed to get Sender DID")
					return nil
				}
			}
			if cmdCfg.receiverAddr == "" {
				cmdCfg.log.Info("Receiver address cannot be empty")
				fmt.Print("Enter Receiver DID : ")
				_, err := fmt.Scan(&cmdCfg.receiverAddr)
				if err != nil {
					cmdCfg.log.Error("Failed to get Receiver DID")
					return nil
				}
			}
			is_alphanumeric_sender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.senderAddr)
			is_alphanumeric_receiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.receiverAddr)
			if !is_alphanumeric_sender || !is_alphanumeric_receiver {
				cmdCfg.log.Error("Invalid sender or receiver address. Please provide valid DID")
				return nil
			}
			if !strings.HasPrefix(cmdCfg.senderAddr, "bafybmi") || len(cmdCfg.senderAddr) != 59 || !strings.HasPrefix(cmdCfg.receiverAddr, "bafybmi") || len(cmdCfg.receiverAddr) != 59 {
				cmdCfg.log.Error("Invalid sender or receiver DID")
				return nil
			}
			if cmdCfg.rbtAmount < 0.00001 {
				cmdCfg.log.Error("Invalid RBT amount. RBT amount should be atlease 0.00001")
				return nil
			}
			if cmdCfg.transType < 1 || cmdCfg.transType > 2 {
				cmdCfg.log.Error("Invalid trans type. TransType should be 1 or 2")
				return nil
			}

			rt := model.RBTTransferRequest{
				Receiver:   cmdCfg.receiverAddr,
				Sender:     cmdCfg.senderAddr,
				TokenCount: cmdCfg.rbtAmount,
				Type:       cmdCfg.transType,
				Comment:    cmdCfg.transComment,
			}

			br, err := cmdCfg.c.TransferRBT(&rt)
			if err != nil {
				cmdCfg.log.Error("Failed RBT transfer", "err", err)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				cmdCfg.log.Error("Failed to trasnfer RBT", "msg", msg)
				return nil
			}
			cmdCfg.log.Info(msg)
			cmdCfg.log.Info("RBT transfered successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.senderAddr, "senderAddr", "", "Sender address")
	cmd.Flags().StringVar(&cmdCfg.receiverAddr, "receiverAddr", "", "Receiver address")
	cmd.Flags().Float64Var(&cmdCfg.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	cmd.Flags().StringVar(&cmdCfg.transComment, "transComment", "", "Transaction comment")
	cmd.Flags().IntVar(&cmdCfg.transType, "transType", 2, "Transaction type")

	return cmd
}

func selfTransferRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self-transfer",
		Short: "Self transfer RBT tokens",
		Long:  "Self transfer RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.senderAddr == "" {
				cmdCfg.log.Info("Sender address cannot be empty")
				fmt.Print("Enter Sender DID : ")
				_, err := fmt.Scan(&cmdCfg.senderAddr)
				if err != nil {
					cmdCfg.log.Error("Failed to get Sender DID")
					return nil
				}
			}
			is_alphanumeric_sender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.senderAddr)
			if !is_alphanumeric_sender {
				cmdCfg.log.Error("Invalid sender or receiver address. Please provide valid DID")
				return nil
			}
			if !strings.HasPrefix(cmdCfg.senderAddr, "bafybmi") || len(cmdCfg.senderAddr) != 59 {
				cmdCfg.log.Error("Invalid sender or receiver DID")
				return nil
			}
			if cmdCfg.transType < 1 || cmdCfg.transType > 2 {
				cmdCfg.log.Error("Invalid trans type. TransType should be 1 or 2")
				return nil
			}
			rt := model.RBTTransferRequest{
				Receiver: cmdCfg.senderAddr,
				Sender:   cmdCfg.senderAddr,
				Type:     cmdCfg.transType,
			}

			br, err := cmdCfg.c.SelfTransferRBT(&rt)
			if err != nil {
				cmdCfg.log.Error("Failed Self RBT transfer", "err", err)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				cmdCfg.log.Error("Failed to self trasnfer RBT", "msg", msg)
				return nil
			}
			cmdCfg.log.Info(msg)
			cmdCfg.log.Info("Self RBT transfered successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.senderAddr, "senderAddr", "", "Sender address")
	cmd.Flags().IntVar(&cmdCfg.transType, "transType", 2, "Transaction type")

	return cmd
}

func generateTestRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-test-tokens",
		Short: "Generate Test RBT tokens",
		Long:  "Generate Test RBT tokens",
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
			if cmdCfg.numTokens <= 0 {
				cmdCfg.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
				return nil
			}

			br, err := cmdCfg.c.GenerateTestRBT(cmdCfg.numTokens, cmdCfg.did)
			if err != nil {
				cmdCfg.log.Error("Failed to generate RBT", "err", err)
				return nil
			}
			if !br.Status {
				cmdCfg.log.Error("Failed to generate RBT", "msg", br.Message)
				return nil
			}

			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				cmdCfg.log.Error("Failed to generate test RBT, " + msg)
				return nil
			}
			cmdCfg.log.Info("Test RBT generated successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")
	cmd.Flags().IntVar(&cmdCfg.numTokens, "numTokens", 1, "Number of tokens")

	return cmd
}
