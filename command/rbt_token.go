package command

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/spf13/cobra"
)

func transferRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Transfer RBT tokens",
		Long:  "Transfer RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
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

func generateTestRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-test-tokens",
		Short: "Generate Test RBT tokens",
		Long:  "Generate Test RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
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

