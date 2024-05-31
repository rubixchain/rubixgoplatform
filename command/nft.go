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

func nftCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "nft",
		Short: "NFT related subcommands",
		Long: "NFT related subcommands",
	}

	cmd.AddCommand(
		createNFT(cmdCfg),
		getAllNFTs(cmdCfg),
	)

	return cmd
}

func createNFT(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		Short: "Create an NFT",
		Long: "Create an NFT",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Error("Failed to create NFT, DID is required to create NFT")
				return nil
			}
			nt := client.CreateNFTReq{
				NumTokens: cmdCfg.numTokens,
				DID:       cmdCfg.did,
				UserID:    cmdCfg.userID,
				UserInfo:  cmdCfg.userInfo,
			}
			if cmdCfg.fileMode {
				nt.Files = make([]string, 0)
				nt.Files = append(nt.Files, cmdCfg.file)
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
				nt.FileInfo = string(jb)
			}
			br, err := cmdCfg.c.CreateNFT(&nt)
			if err != nil {
				cmdCfg.log.Error("Failed to create NFT", "err", err)
				return nil
			}
			if !br.Status {
				cmdCfg.log.Error("Failed to create NFT", "msg", br.Message)
				return nil
			}
			msg, status := signatureResponse(cmdCfg, br)
			if !status {
				cmdCfg.log.Error("Failed to create NFT, " + msg)
				return nil
			}
			cmdCfg.log.Info(fmt.Sprintf("Data Token : %s", msg))
			cmdCfg.log.Info("NFT created successfully")
			return nil
		},
	}

	cmd.Flags().BoolVar(&cmdCfg.fileMode, "fmode", false, "File mode")
	cmd.Flags().StringVar(&cmdCfg.file, "file", "file.txt", "File to be uploaded")
	cmd.Flags().StringVar(&cmdCfg.userID, "uid", "testuser", "User ID for token creation")
	cmd.Flags().StringVar(&cmdCfg.userInfo, "uinfo", "", "User info for token creation")
	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")
	cmd.Flags().IntVar(&cmdCfg.numTokens, "numTokens", 1, "Number of tokens")

	return cmd
}

func getAllNFTs(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Short: "List NFTs by DID",
		Long: "List NFTs by DID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Error("Failed to get NFTs, DID is required to get NFTs")
				return nil
			}
			tkns, err := cmdCfg.c.GetAllNFTs(cmdCfg.did)
			if err != nil {
				cmdCfg.log.Error("Failed to get NFTs, " + err.Error())
				return nil
			}
			for _, tkn := range tkns.Tokens {
				fmt.Printf("NFT : %s, Status : %d\n", tkn.Token, tkn.TokenStatus)
			}
			cmdCfg.log.Info("Got all NFTs successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")
	
	return cmd
}
