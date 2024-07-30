package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func validateCommand(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-chain",
		Short: "Validate Token or Smart Contract chain",
		Long:  "Validate Token or Smart Contract chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Info("Tokenchain-validator did cannot be empty")
				fmt.Print("Enter tokenchain-validator DID : ")
				_, err := fmt.Scan(&cmdCfg.did)
				if err != nil {
					cmdCfg.log.Error("Failed to get tokenchain-validator DID")
					return nil
				}
			}

			br, err := cmdCfg.c.ValidateTokenchain(cmdCfg.did, cmdCfg.smartContractChainValidation, cmdCfg.token, cmdCfg.blockCount)
			if err != nil {
				cmdCfg.log.Error("failed to validate token chain", "err", err)
				return nil
			}

			if !br.Status {
				cmdCfg.log.Error("failed to validate token chain", "msg", br.Message)
				return nil
			}

			cmdCfg.log.Info("Tokenchain validated successfully", "msg", br.Message)
			return nil
		},
	}

	cmd.Flags().IntVar(&cmdCfg.blockCount, "blockCount", 0, "Number of blocks of the tokenchain to validate")
	cmd.Flags().BoolVar(&cmdCfg.smartContractChainValidation, "sctValidation", false, "Validate smart contract token chain")
	cmd.Flags().StringVar(&cmdCfg.token, "tokenHash", "", "Token Hash")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}
