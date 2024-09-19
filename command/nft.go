package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
)

func (cmd *Command) createNFT() {
	if cmd.did == "" {
		cmd.log.Error("Failed to create NFT, DID is required to create NFT")
		return
	}

	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}

	if cmd.nftFileInfo == "" {
		cmd.log.Error("Failed to create NFT, NFT file Info is required to create NFT")
		return
	}
	// nftRequest := core.NFTReq{
	// 	DID:         cmd.did,
	// 	UserID:      cmd.userID,
	// 	NFTFileInfo: cmd.nftFileInfo,
	// 	NFTFile:     cmd.nftFilePath,
	// }

	request := client.CreateNFTReq{
		DID:         cmd.did,
		UserID:      cmd.userID,
		NFTFileInfo: cmd.nftFileInfo,
		NFTFile:     cmd.nftFilePath,
	}

	br, err := cmd.c.CreateNFT(&request)
	if err != nil {
		cmd.log.Error("Failed to create NFT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to create NFT", "msg", br.Message)
		return
	}
	cmd.log.Info(fmt.Sprintf("NFT info : %s", br.Message))
	cmd.log.Info("NFT created successfully")
}

func (cmd *Command) SubscribeNFT() {
	if cmd.nft == "" {
		cmd.log.Info("nft id cannot be empty")
		fmt.Print("Enter nft id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get nft")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.nft)
	if len(cmd.nft) != 46 || !strings.HasPrefix(cmd.nft, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid in subscribe nft ")
		return
	}

	basicResponse, err := cmd.c.SubscribeNFT(cmd.nft)

	if err != nil {
		cmd.log.Error("Failed to subscribe nft", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to subscribe nft", "msg", basicResponse.Message)
		return
	}
	message, status := cmd.SignatureResponse(basicResponse)

	if !status {
		cmd.log.Error("Failed to subscribe nft, " + message)
		return
	}
	cmd.log.Info("New event subscribed successfully")
}

func (cmd *Command) getAllNFTs() {
	if cmd.did == "" {
		cmd.log.Error("Failed to get NFTs, DID is required to get NFTs")
		return
	}
	tkns, err := cmd.c.GetAllNFTs(cmd.did)
	if err != nil {
		cmd.log.Error("Failed to get NFTs, " + err.Error())
		return
	}
	for _, tkn := range tkns.Tokens {
		fmt.Printf("NFT : %s, Status : %d\n", tkn.Token, tkn.TokenStatus)
	}
	cmd.log.Info("Got all NFTs successfully")
}
