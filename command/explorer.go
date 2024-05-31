package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func explorerCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "explorer",
		Short: "Explorer related commands",
		Long: "Explorer related commands",
	}

	cmd.AddCommand(
		addExplorer(cmdCfg),
		removeExplorer(cmdCfg),
		getAllExplorer(cmdCfg),
	)

	return cmd
}

const linksFlag string = "links"

func addExplorer(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add",
		Short: "Add Explorer URLs",
		Long: "Add Explorer URLs",
		RunE: func(cmd *cobra.Command, args []string) error {
			links, err := cmd.Flags().GetStringSlice(linksFlag)
			if err != nil {
				return err
			}

			if len(links) == 0 {
				cmdCfg.log.Error("links are required for Explorer")
				return nil
			}
			msg, status := cmdCfg.c.AddExplorer(links)
		
			if !status {
				cmdCfg.log.Error("Add Explorer command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Add Explorer command finished, " + msg)
				return nil
			}
		},
	}

	cmd.Flags().StringSlice(linksFlag, []string{}, "Explorer URLs")

	return cmd
}

func removeExplorer(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "remove",
		Short: "Remove Explorer URLs",
		Long: "Remove Explorer URLs",
		RunE: func(cmd *cobra.Command, args []string) error {
			links, err := cmd.Flags().GetStringSlice(linksFlag)
			if err != nil {
				return err
			}

			if len(links) == 0 {
				cmdCfg.log.Error("links required for Explorer")
				return nil
			}

			msg, status := cmdCfg.c.RemoveExplorer(links)
			if !status {
				cmdCfg.log.Error("Remove Explorer command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Remove Explorer command finished, " + msg)
				return nil
			}
		},
	}

	cmd.Flags().StringSlice(linksFlag, []string{}, "Explorer URLs")

	return cmd
}

func getAllExplorer(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: "List all Explorer URLs",
		Long: "List all Explorer URLs",
		RunE: func(cmd *cobra.Command, args []string) error {
			links, msg, status := cmdCfg.c.GetAllExplorer()
			if !status {
				cmdCfg.log.Error("Get all Explorer command failed, " + msg)
				return nil
			} else {
				cmdCfg.log.Info("Get all Explorer command finished, " + msg)
				cmdCfg.log.Info("Explorer links", "links", links)
				for i, q := range links {
					fmt.Printf("URL %d: %s\n", i, q)
				}
				return nil
			}
		},
	}

	return cmd
}