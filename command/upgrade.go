package command

import (
	"errors"

	"github.com/spf13/cobra"
)

func upgradeGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade related subcommands",
		Long:  "Upgrade related subcommands",
	}

	cmd.AddCommand(
		unpledgePOWBasedPledgedTokensCmd(cmdCfg),
	)

	return cmd
}

func unpledgePOWBasedPledgedTokensCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpledge-pow-tokens",
		Long:  "Unpledge any pledge tokens which were pledged as part of PoW based pledging",
		Short: "Unpledge any pledge tokens which were pledged as part of PoW based pledging",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCfg.log.Info("Unpledging of POW-based pledged tokens has started")
			msg, status := cmdCfg.c.UnpledgePOWBasedPledgedTokens()
			if !status {
				cmdCfg.log.Error(msg)
				return errors.New(msg)
			}

			cmdCfg.log.Info(msg)
			return nil
		},
	}

	return cmd
}