package command

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func quorumCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quorum",
		Short: "Quorum related subcommands",
		Long:  "Quorum related subcommands",
	}

	cmd.AddCommand(
		addQuorumCmd(cmdCfg),
		listQuorumsCmd(cmdCfg),
		removeAllQuorumCmd(cmdCfg),
		setupQuorumCmd(cmdCfg),
		runUnpledgeCmd(cmdCfg),
		getPledgedTokenStateDetailsCmd(cmdCfg),
	)

	return cmd
}

func getPledgedTokenStateDetailsCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-token-states",
		Short: "List all pledge token states of a node",
		Long:  "List all pledge token states of a node",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info, err := cmdCfg.c.GetPledgedTokenDetails()
			if err != nil {
				cmdCfg.log.Error("Invalid response from the node", "err", err)
				return err
			}
			fmt.Printf("Response : %v\n", info)
			if !info.Status {
				cmdCfg.log.Error("Failed to get account info", "message", info.Message)
			} else {
				cmdCfg.log.Info("Successfully got the pledged token states info")
				fmt.Println("DID	", "Pledged Token	", "Token State")
				for _, i := range info.PledgedTokenStateDetails {
					fmt.Println(i.DID, "	", i.TokensPledged, "	", i.TokenStateHash)
				}
			}
			return nil
		},
	}

	return cmd
}

func runUnpledgeCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpledge",
		Short: "Unpledge all pledged tokens",
		Long:  "Unpledge all pledged tokens",
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, status := cmdCfg.c.RunUnpledge()
			cmdCfg.log.Info("Unpledging of pledged tokens has started")
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

func addQuorumCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add addresses present with quorumlist.json in the node",
		Long:  "Add addresses present with quorumlist.json in the node",
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
		Use:   "list",
		Short: "List all Quorums",
		Long:  "List all Quorums",
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
		Use:   "remove-all",
		Short: "Remove all Quorums",
		Long:  "Remove all Quorums",
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
		Use:   "setup",
		Short: "Setup up DID as a Quorum",
		Long:  "Setup up DID as a Quorum",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Info("Quorum DID cannot be empty")
				fmt.Print("Enter Quorum DID : ")
				_, err := fmt.Scan(&cmdCfg.did)
				if err != nil {
					cmdCfg.log.Error("Failed to get Quorum DID")
					return nil
				}
			}
			is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.did)
			if !strings.HasPrefix(cmdCfg.did, "bafybmi") || len(cmdCfg.did) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid DID")
				return nil
			}

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
