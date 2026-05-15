package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
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
	nftBalance, err := util.ExtractResult[[]types.NFTBalance](tkns.Result)
	if err != nil {
		cmd.log.Error("failed to parse nft balance")
		return
	}
	if len(nftBalance) == 0 {
		cmd.log.Info("No NFTs found")
		return
	}
	cmd.log.Info("Got NFTs balance successfully")
	cmd.log.Info("DID", cmd.did)
	for _, nft := range nftBalance {
		fmt.Printf("NFT id : %s, Value :  %10.*f\n", nft.NFTId, constants.MaxSupportedDecimalPlaces, nft.NFTValue)
	}
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

func (cmd *Command) DeployNFT() {
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
	if cmd.deployerAddr == "" {
		cmd.log.Info("Deployer address cannot be empty")
		fmt.Print("Enter Deployer DID : ")
		_, err := fmt.Scan(&cmd.deployerAddr)
		if err != nil {
			cmd.log.Error("Failed to get deployer DID")
			return
		}
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.deployerAddr)
	if !strings.HasPrefix(cmd.deployerAddr, "bafybmi") || len(cmd.deployerAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid deployer DID")
		return
	}

	if cmd.nftValue < 0.001 {
		cmd.log.Error("Invalid NFT value. NFT value should be atlease 0.001")
		return
	}

	nftInfo := models.NFTInfo{
		NFTId: cmd.nft,
		Value: cmd.nftValue,
		Data:  cmd.nftData,
	}
	nftDeployRequest := models.TransactionRequest{
		Initiator: cmd.deployerAddr,
		Tokens: models.TransactionTokenDetails{
			NFT: []models.NFTInfo{nftInfo},
		},
		Memo: cmd.transComment,
	}

	br, err := cmd.c.InitiateTransaction(&nftDeployRequest)
	if err != nil {
		cmd.log.Error("Failed NFT deployment", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to deployed NFT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("NFT deployed successfully")
}

func (cmd *Command) ExecuteNFT() {
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
	if cmd.executorAddr == "" {
		cmd.log.Info("Executor address cannot be empty")
		fmt.Print("Enter Executor DID : ")
		_, err := fmt.Scan(&cmd.executorAddr)
		if err != nil {
			cmd.log.Error("Failed to get Executor DID")
			return
		}
	}
	is_alphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.executorAddr)
	if !strings.HasPrefix(cmd.executorAddr, "bafybmi") || len(cmd.executorAddr) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid Executor DID")
		return
	}

	if cmd.nftValue < 0.001 {
		cmd.log.Error("Invalid NFT value. NFT value should be atlease 0.001")
		return
	}

	nftInfo := models.NFTInfo{
		NFTId: cmd.nft,
		Value: cmd.nftValue,
		Data:  cmd.nftData,
	}
	nftExecuteRequest := models.TransactionRequest{
		Initiator: cmd.executorAddr,
		Owner:     cmd.receiverAddr,
		Tokens: models.TransactionTokenDetails{
			NFT:                  []models.NFTInfo{nftInfo},
			TransferNFTOwnership: cmd.isNftTransfer,
		},
		Memo: cmd.transComment,
	}

	br, err := cmd.c.InitiateTransaction(&nftExecuteRequest)
	if err != nil {
		cmd.log.Error("Failed NFT execution", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to execute NFT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("NFT executed successfully")
}
