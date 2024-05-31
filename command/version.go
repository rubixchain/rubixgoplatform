package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	version = "0.0.17"
)

func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "version",
		Short: "Rubix version",
		Long: "Rubix version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
			if err != nil {
				return fmt.Errorf("failed to fetch Rubix version, err: %v", err.Error())
			}
			return nil
		},
	}

	return cmd
}