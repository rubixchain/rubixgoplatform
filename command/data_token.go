package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/spf13/cobra"
)

func createDataToken(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Data Token",
		Long:  "Create a Data Token",
		RunE: func(cmd *cobra.Command, args []string) error {
			dt := client.DataTokenReq{
				DID:      cmdCfg.did,
				UserID:   cmdCfg.userID,
				UserInfo: cmdCfg.userInfo,
			}

			if cmdCfg.fileMode {
				dt.Files = make([]string, 0)
				dt.Files = append(dt.Files, cmdCfg.file)
			} else {
				fd, err := ioutil.ReadFile(cmdCfg.file)
				if err != nil {
					cmdCfg.log.Error("Failed to read file", "err", err)
					return nil
				}
				hb := util.CalculateHash(fd, "SHA3-256")
				fi := make(map[string]map[string]string)
				fn := path.Base(cmdCfg.file)
				info := make(map[string]string)
				info[core.DTFileHashField] = util.HexToStr(hb)
				fi[fn] = info
				jb, err := json.Marshal(fi)
				if err != nil {
					cmdCfg.log.Error("Failed to marshal json input", "err", err)
					return nil
				}
				dt.FileInfo = string(jb)
			}
			br, err := cmdCfg.c.CreateDataToken(&dt)
			if err != nil {
				cmdCfg.log.Error("Failed to create data token", "err", err)
				return nil
			}
			if !br.Status {
				cmdCfg.log.Error("Failed to create data token", "msg", br.Message)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				cmdCfg.log.Error("Failed to create data token, " + msg)
				return nil
			}
			cmdCfg.log.Info(fmt.Sprintf("Data Token : %s", msg))
			cmdCfg.log.Info("Data token created successfully")
			return nil
		},
	}

	cmd.Flags().BoolVar(&cmdCfg.fileMode, "fmode", false, "File mode")
	cmd.Flags().StringVar(&cmdCfg.file, "file", "file.txt", "File to be uploaded")
	cmd.Flags().StringVar(&cmdCfg.userID, "uid", "testuser", "User ID for token creation")
	cmd.Flags().StringVar(&cmdCfg.userInfo, "uinfo", "", "User info for token creation")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}

func commitDataToken(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit a Data Token",
		Long:  "Commit a Data Token",
		RunE: func(cmd *cobra.Command, args []string) error {
			br, err := cmdCfg.c.CommitDataToken(cmdCfg.did, cmdCfg.batchID)
			if err != nil {
				cmdCfg.log.Error("Failed to commit data token", "err", err)
				return nil
			}

			if !br.Status {
				cmdCfg.log.Error("Failed to commit data token", "msg", br.Message)
				return nil
			}

			msg, status := signatureResponse(cmdCfg, br)

			if !status {
				cmdCfg.log.Error("Failed to commit data token, " + msg)
				return nil
			}

			cmdCfg.log.Info("Data tokens committed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")
	cmd.Flags().StringVar(&cmdCfg.batchID, "bid", "batchID1", "Batch ID")

	return cmd
}
