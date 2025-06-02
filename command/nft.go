package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
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

	if cmd.metadata == "" {
		cmd.log.Error("Failed to create NFT, NFT metadata is required to create NFT")
		return
	}

	if cmd.artifact == "" {
		cmd.log.Error("Failed to create NFT, NFT artifact is required to create NFT")
		return
	}

	request := client.CreateNFTReq{
		DID:      cmd.did,
		Metadata: cmd.metadata,
		Artifact: cmd.artifact,
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

func (cmd *Command) deployNFT() {
	if cmd.nft == "" {
		cmd.log.Info("NFT id cannot be empty")
		fmt.Print("Enter NFT Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get NFT")
			return
		}
	}
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.nft)
	if len(cmd.nft) != 46 || !strings.HasPrefix(cmd.nft, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid NFT")
		return
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.deployerAddr)
	if !strings.HasPrefix(cmd.deployerAddr, "bafybmi") || len(cmd.deployerAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid deployer DID")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type")
		return
	}
	deployRequest := model.DeployNFTRequest{
		NFT:        cmd.nft,
		DID:        cmd.deployerAddr,
		QuorumType: cmd.transType,
		NFTValue:   cmd.nftValue,
		NFTData:    cmd.nftData,
	}
	response, err := cmd.c.DeployNFT(&deployRequest)
	if err != nil {
		cmd.log.Error("Failed to deploy NFT, Token ", cmd.nft, "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(response)
	if !status {
		cmd.log.Error("Failed to deploy NFT, Token ", cmd.nft, "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("NFT Deployed successfully")
}

func (cmd *Command) executeNFT() {
	if cmd.nft == "" {
		cmd.log.Info("NFT id cannot be empty")
		fmt.Print("Enter NFT Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get SC Token ID")
			return
		}
	}

	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.nft)
	if len(cmd.nft) != 46 || !strings.HasPrefix(cmd.nft, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid nft")
		return
	}

	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.executorAddr)
	if !strings.HasPrefix(cmd.executorAddr, "bafybmi") || len(cmd.executorAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid executer DID")
		return
	}
	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type")
		return
	}

	executeRequest := model.ExecuteNFTRequest{
		NFT:        cmd.nft,
		Executor:   cmd.executorAddr,
		Receiver:   cmd.receiverAddr,
		QuorumType: cmd.transType,
		Comment:    cmd.transComment,
		NFTValue:   cmd.rbtAmount,
	}
	response, err := cmd.c.ExecuteNFT(&executeRequest)
	if err != nil {
		cmd.log.Error("Failed to execute NFT, Token ", cmd.nft, "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(response)
	if !status {
		cmd.log.Error("Failed to execute nft, Token ", cmd.nft, "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("NFT executed successfully")

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

func (cmd *Command) getNFTsByDid() {
	if cmd.did == "" {
		cmd.log.Error("Failed to get NFTs, DID is required to get NFTs")
		return
	}
	tkns, err := cmd.c.GetNFTsByDid(cmd.did)
	if err != nil {
		cmd.log.Error("Failed to get NFTs, " + err.Error())
		return
	}
	for _, tkn := range tkns.Tokens {
		fmt.Printf("NFT : %s, Status : %d\n", tkn.Token, tkn.TokenStatus)
	}
	cmd.log.Info("Got all NFTs successfully")
}

func (cmd *Command) fetchNFT() {
	if cmd.nft == "" {
		cmd.log.Info("nft id cannot be empty")
		fmt.Print("Enter NFT Token Id : ")
		_, err := fmt.Scan(&cmd.nft)
		if err != nil {
			cmd.log.Error("Failed to get NFT Token ID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.nft)

	if len(cmd.nft) != 46 || !strings.HasPrefix(cmd.nft, "Qm") || !isAlphanumeric {
		cmd.log.Error("Invalid smart contract token")
		return
	}
	nftRequest := core.FetchNFTRequest{
		NFT: cmd.nft,
	}

	request := client.FetchNFTRequest{
		NFT: nftRequest.NFT,
	}

	basicResponse, err := cmd.c.FetchNFT(&request)
	if err != nil {
		cmd.log.Error("Failed to fetch nft", "err", err)
		return
	}
	if !basicResponse.Status {
		cmd.log.Error("Failed to fetch nft", "err", err)
		return
	}
	cmd.log.Info("NFT fetched successfully")
}
