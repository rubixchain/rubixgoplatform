package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func quorumCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "quorum",
		Short: "Quorum related subcommands",
		Long: "Quorum related subcommands",
	}

	cmd.AddCommand(
		addQuorumCmd(cmdCfg),
		listQuorumsCmd(cmdCfg),
		removeAllQuorumCmd(cmdCfg),
		setupQuorumCmd(cmdCfg),
	)

	return cmd
}

func addQuorumCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add",
		Short: "Add addresses present with quorumlist.json in the node",
		Long: "Add addresses present with quorumlist.json in the node",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, status := cmdCfg.c.AddQuorum(cmdCfg.quorumList)
			if !status {
				cmdCfg.log.Error("Failed to add quorum list to node", "msg", msg)
				return nil
			}
			cmdCfg.log.Info("Quorum list added successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.quorumList, "quorumList", "quorumlist.json", "Quorum list")

	return cmd
}

func listQuorumsCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: "List all Quorums",
		Long: "List all Quorums",
		RunE: func(cmd *cobra.Command, args []string) error {
			response, err := cmdCfg.c.GettAllQuorum()
			if err != nil {
				cmdCfg.log.Error("Invalid response from the node", "err", err)
				return nil
			}
			if !response.Status {
				cmdCfg.log.Error("Failed to get quorum list from node", "msg", response.Message)
				return nil
			}
			for _, q := range response.Result {
				fmt.Printf("Address : %s\n", q)
			}
			cmdCfg.log.Info("Got all quorum list successfully")
			return nil
		},
	}

	return cmd
}

func removeAllQuorumCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "remove-all",
		Short: "Remove all Quorums",
		Long: "Remove all Quorums",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, status := cmdCfg.c.RemoveAllQuorum()
			if !status {
				cmdCfg.log.Error("Failed to remove quorum list", "msg", msg)
				return nil
			}
			cmdCfg.log.Info(msg)
			return nil
		},
	}

	return cmd
}

func setupQuorumCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "setup",
		Short: "Setup up DID as a Quorum",
		Long: "Setup up DID as a Quorum",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.forcePWD {
				pwd, err := getpassword("Enter quorum key password: ")
				if err != nil {
					cmdCfg.log.Error("Failed to get password")
					return nil
				}
				cmdCfg.quorumPWD = pwd
			}
			msg, status := cmdCfg.c.SetupQuorum(cmdCfg.did, cmdCfg.quorumPWD, cmdCfg.privPWD)
		
			if !status {
				cmdCfg.log.Error("Failed to setup quorum", "msg", msg)
				return nil
			}
			cmdCfg.log.Info("Quorum setup successfully")
			return nil
		},
	}

	cmd.Flags().BoolVar(&cmdCfg.forcePWD, "fp", false, "Force password entry")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}