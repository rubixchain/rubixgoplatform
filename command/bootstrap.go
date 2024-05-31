package command

import (
	"errors"

	"github.com/spf13/cobra"
)

const peersFlag string = "peers"

func bootstrapCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap related subcommands",
		Long:  "Bootstrap related subcommands",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		addBootStrap(cmdCfg),
		removeBootStrap(cmdCfg),
		removeAllBootStrap(cmdCfg),
		getAllBootStrap(cmdCfg),
	)

	return cmd
}

func addBootStrap(cmdCfg *CommandConfig) *cobra.Command {	
	cmd := &cobra.Command{
		Use: "add",
		Short: "Add IPFS bootstrap peers",
		Long: "Add IPFS bootstrap peers",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			peers, err := cmd.Flags().GetStringArray(peersFlag)
			if err != nil {
				return err
			}

			if len(peers) == 0 {
				errMsg := errors.New("peers are required for bootstrap")
				cmdCfg.log.Error(errMsg.Error())
				return nil
			}
			msg, status := cmdCfg.c.AddBootStrap(peers)
		
			if !status {
				cmdCfg.log.Error("Add bootstrap command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Add bootstrap command finished, " + msg)
				return nil
			}
		},
	}

	cmd.Flags().StringSlice(peersFlag, []string{}, "Bootstrap peers, mutiple peers will be seprated by comma")

	return cmd
}

func removeBootStrap(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "remove",
		Short: "Remove bootstrap peer(s) from the configuration",
		Long: "Remove bootstrap peer(s) from the configuration",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			peers, err := cmd.Flags().GetStringArray(peersFlag)
			if err != nil {
				return err
			}

			if len(peers) == 0 {
				cmdCfg.log.Error("Peers required for bootstrap")
				return nil
			}

			msg, status := cmdCfg.c.RemoveBootStrap(peers)
			if !status {
				cmdCfg.log.Error("Remove bootstrap command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Remove bootstrap command finished, " + msg)
				return nil
			}
		},
	}
	
	cmd.Flags().StringSlice(peersFlag, []string{}, "Bootstrap peers, mutiple peers will be seprated by comma")

	return cmd
}

func removeAllBootStrap(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "remove-all",
		Short: "Removes all bootstrap peers from the configuration",
		Long: "Removes all bootstrap peers from the configuration",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, status := cmdCfg.c.RemoveAllBootStrap()
			if !status {
				cmdCfg.log.Error("Remove all bootstrap command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Remove all bootstrap command finished, " + msg)
				return nil
			}
		},
	}

	return cmd
}

func getAllBootStrap(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: "List all bootstrap peers from the configuration",
		Long: "List all bootstrap peers from the configuration",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			peers, msg, status := cmdCfg.c.GetAllBootStrap()
			if !status {
				cmdCfg.log.Error("Get all bootstrap command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Get all bootstrap command finished, " + msg)
				cmdCfg.log.Info("Bootstrap peers", "peers", peers)
				return nil
			}
		},
	}

	return cmd
}
