package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/spf13/cobra"
)

func nodeGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Node related subcommands",
		Long:  "Node related subcommands",
	}

	cmd.AddCommand(
		shutDownCmd(cmdCfg),
		ping(cmdCfg),
		checkQuorumStatus(cmdCfg),
		peerIDCmd(cmdCfg),
		migrateNodeCmd(cmdCfg),
		lockRBTTokensCmd(cmdCfg),
		releaseAllLockedRBTTokensCmd(cmdCfg),
	)

	return cmd
}

func peerIDCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peer-id",
		Short: "Get the IPFS peer id",
		Long:  "Get the IPFS peer id",
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, status := cmdCfg.c.PeerID()
			if !status {
				cmdCfg.log.Error("Failed to fetch peer ID of the node", "msg", msg)
				return nil
			}
			_, err := fmt.Fprint(os.Stdout, msg, "\n")
			if err != nil {
				cmdCfg.log.Error(err.Error())
				return nil
			}
			return nil
		},
	}

	return cmd
}

func shutDownCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shutdown",
		Short: "shut down the node",
		Long:  "shut down the node",
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, status := cmdCfg.c.Shutdown()
			if !status {
				cmdCfg.log.Error("Failed to shutdown", "msg", msg)
				return nil
			}
			cmdCfg.log.Info("Shutdown initiated successfully, " + msg)
			return nil
		},
	}

	return cmd
}

func ping(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "pings a peer",
		Long:  "pings a peer",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, status := cmdCfg.c.Ping(cmdCfg.peerID)
			if !status {
				cmdCfg.log.Error("Ping failed", "message", msg)
				return nil
			} else {
				cmdCfg.log.Info("Ping response received successfully", "message", msg)
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&cmdCfg.peerID, "peerID", "", "Peerd ID")

	return cmd
}

func checkQuorumStatus(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quorum-status",
		Long:  "check the status of quorum",
		Short: "check the status of quorum",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, _ := cmdCfg.c.CheckQuorumStatus(cmdCfg.quorumAddr)
			//Verification with "status" pending !
			if strings.Contains(msg, "Quorum is setup") {
				cmdCfg.log.Info("Quorum is setup in", cmdCfg.quorumAddr, "message", msg)
				return nil
			} else {
				cmdCfg.log.Error("Quorum is not setup in ", cmdCfg.quorumAddr, " message ", msg)
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&cmdCfg.quorumAddr, "quorumAddr", "", "Quorum Node Address to check the status of the Quorum")

	return cmd
}

func migrateNodeCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate Node",
		Long:  "Migrate Node",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.forcePWD {
				pwd, err := getpassword("Set private key password: ")
				if err != nil {
					cmdCfg.log.Error("Failed to get password")
					return nil
				}
				npwd, err := getpassword("Re-enter private key password: ")
				if err != nil {
					cmdCfg.log.Error("Failed to get password")
					return nil
				}
				if pwd != npwd {
					cmdCfg.log.Error("Password mismatch")
					return nil
				}
				cmdCfg.privPWD = pwd
			}

			if cmdCfg.forcePWD {
				pwd, err := getpassword("Set quorum key password: ")
				if err != nil {
					cmdCfg.log.Error("Failed to get password")
					return nil
				}
				npwd, err := getpassword("Re-enter quorum key password: ")
				if err != nil {
					cmdCfg.log.Error("Failed to get password")
					return nil
				}
				if pwd != npwd {
					cmdCfg.log.Error("Password mismatch")
					return nil
				}
				cmdCfg.quorumPWD = pwd
			}

			r := core.MigrateRequest{
				DIDType:   cmdCfg.didType,
				PrivPWD:   cmdCfg.privPWD,
				QuorumPWD: cmdCfg.quorumPWD,
			}
			br, err := cmdCfg.c.MigrateNode(&r, cmdCfg.timeout)
			if err != nil {
				cmdCfg.log.Error("Failed to migrate node", "err", err)
				return nil
			}
			if !br.Status {
				cmdCfg.log.Error("Failed to migrate node", "msg", br.Message)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, br, cmdCfg.timeout)
			if !status {
				cmdCfg.log.Error("Failed to migrate node, " + msg)
				return nil
			}
			cmdCfg.log.Info("Node migrated successfully, " + msg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&cmdCfg.forcePWD, "fp", false, "Force password entry")
	cmd.Flags().StringVar(&cmdCfg.privPWD, "privPWD", "mypassword", "Private key password")
	cmd.Flags().StringVar(&cmdCfg.quorumPWD, "quorumPWD", "mypassword", "Quorum key password")
	cmd.Flags().IntVar(&cmdCfg.didType, "didType", 0, "DID Creation type")
	cmd.Flags().DurationVar(&cmdCfg.timeout, "timeout", 0, "Timeout for the server")

	return cmd
}

func lockRBTTokensCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock-rbt-tokens",
		Short: "Lock RBT tokens",
		Long:  "Lock RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			fb, err := ioutil.ReadFile(cmdCfg.tokenList)
			if err != nil {
				cmdCfg.log.Error("Failed to read token list", "err", err)
				return nil
			}
			var ts []string
			err = json.Unmarshal(fb, &ts)
			if err != nil {
				cmdCfg.log.Error("Invalid token list", "err", err)
				return nil
			}
			br, err := cmdCfg.c.LockToknes(ts)
			if err != nil {
				cmdCfg.log.Error("Failed to lock tokens", "err", err)
				return nil
			}
			if !br.Status {
				cmdCfg.log.Error("Failed to lock tokens", "msg", br.Message)
				return nil
			}
			cmdCfg.log.Info("Tokens locked sucessfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.tokenList, "tokenList", "tokens.txt", "Token list")

	return cmd
}


func releaseAllLockedRBTTokensCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "release-rbt-tokens",
		Short: "Release all locked RBT tokens",
		Long: "Release all locked RBT tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := cmdCfg.c.ReleaseAllLockedTokens()
			if err != nil {
				cmdCfg.log.Error("Failed to release the locked tokens", "err", err)
				return nil
			}
			if !resp.Status {
				cmdCfg.log.Error("Failed to release the locked tokens", "msg", resp.Message)
				return nil
			}

			cmdCfg.log.Info("Locked Tokens released successfully Or No Locked Tokens found to be released")
			return nil
		},
	}

	return cmd
}
