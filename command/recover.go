package command

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/spf13/cobra"
)

func pinningServiceCommand(commandCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "pin-service",
		Short: "Token pinning and recover related subcommands",
		Long: "Token pinning and recover related subcommands",
	}

	cmd.AddCommand(
		recoverTokenCmd(commandCfg),
		pinRBTCmd(commandCfg),
	)

	return cmd
}

func recoverTokenCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recover",
		Long:  "Recovers the pinned token from the pinning service provider node",
		Short: "Recovers the pinned token from the pinning service provider node",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt := model.RBTRecoverRequest{
				PinningNode: cmdCfg.pinningAddress,
				Sender:      cmdCfg.senderAddr,
				TokenCount:  cmdCfg.rbtAmount,
			}
		
			br, err := cmdCfg.c.RecoverRBT(&rt)
			if err != nil {
				cmdCfg.log.Error("Failed to Recover the Tokens", "err", err)
				return err
			}
			if !br.Status {
				errMsg := fmt.Errorf("failed to recover RBT: " + br.Message)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			} else {
				cmdCfg.log.Info("Recovered RBT: " + br.Message)
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&cmdCfg.pinningAddress, "pinningAddress", "", "Pinning address")
	cmd.Flags().StringVar(&cmdCfg.senderAddr, "senderAddr", "", "Sender Address")
	cmd.Flags().Float64Var(&cmdCfg.rbtAmount, "rbtAmount", 0.0, "RBT amount")

	return cmd
}

func pinRBTCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pin",
		Long:  "Pins a token on a pinning service provider node",
		Short: "Pins a token on a pinning service provider node",
		RunE: func(cmd *cobra.Command, args []string) error {			
			rt := model.RBTPinRequest{
				PinningNode: cmdCfg.pinningAddress,
				Sender:      cmdCfg.senderAddr,
				TokenCount:  cmdCfg.rbtAmount,
				Type:        cmdCfg.transType,
				Comment:     cmdCfg.transComment,
			}

			br, err := cmdCfg.c.PinRBT(&rt)
			if err != nil {
				errMsg := fmt.Errorf("failed to pin the token, err: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return err
			}

			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				errMsg := fmt.Errorf("failed to pin the token, msg: %v", msg)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}
			cmdCfg.log.Info(msg)
			cmdCfg.log.Info("RBT Pinned successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.pinningAddress, "pinningAddress", "", "Pinning address")
	cmd.Flags().StringVar(&cmdCfg.senderAddr, "senderAddr", "", "Sender Address")
	cmd.Flags().Float64Var(&cmdCfg.rbtAmount, "rbtAmount", 0.0, "RBT amount")
	cmd.Flags().StringVar(&cmdCfg.transComment, "transComment", "", "Transaction comment")
	cmd.Flags().IntVar(&cmdCfg.transType, "transType", 2, "Transaction type")

	return cmd
}

