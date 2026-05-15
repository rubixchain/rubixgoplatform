package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

func (cmd *Command) GenerateLocalRBT() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get DID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.numTokens <= 0 {
		cmd.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return
	}

	br, err := cmd.c.GenerateLocalRBT(cmd.numTokens, cmd.did, cmd.startIndex)

	if err != nil {
		cmd.log.Error("Failed to generate RBT", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to generate RBT", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to generate test RBT, " + msg)
		return
	}
	cmd.log.Info("Test RBT generated successfully")
}

func (cmd *Command) GenerateMainnetRBT() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get DID")
			return
		}
	}
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.numTokens <= 0 {
		cmd.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return
	}
	if cmd.startIndex < 1 {
		cmd.log.Error("Invalid startIndex, must be >= 1")
		return
	}

	br, err := cmd.c.GenerateMainnetRBT(cmd.numTokens, cmd.did, cmd.startIndex)
	if err != nil {
		cmd.log.Error("Failed to generate mainnet RBT", "err", err)
		return
	}
	if !br.Status {
		cmd.log.Error("Failed to generate mainnet RBT", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to generate mainnet RBT, " + msg)
		return
	}
	cmd.log.Info("Mainnet RBT generated successfully")
}

func (cmd *Command) ValidateTokenchain() {
	if cmd.did == "" {
		cmd.log.Info("Tokenchain-validator did cannot be empty")
		fmt.Print("Enter tokenchain-validator DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get tokenchain-validator DID")
			return
		}
	}
	br, err := cmd.c.ValidateTokenchain(cmd.did, cmd.smartContractChainValidation, cmd.token, cmd.blockCount)
	if err != nil {
		cmd.log.Error("failed to validate token chain", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("failed to validate token chain", "msg", br.Message)
		return
	}

	cmd.log.Info("Tokenchain validated successfully", "msg", br.Message)
}

func (cmd *Command) GenerateFaucetTestRBT() {
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !is_alphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	if cmd.numTokens <= 0 {
		cmd.log.Error("Invalid RBT amount, tokens generated should be a whole number and greater than 0")
		return
	}
	br, err := cmd.c.GenerateFaucetTestRBT(cmd.numTokens, cmd.did)

	if err != nil {
		cmd.log.Error("Failed to generate RBT", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to generate RBT", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to generate test RBT, " + msg)
		return
	}
	cmd.log.Info("Test RBT generated successfully")
}

func (cmd *Command) FaucetTokenCheck() {
	is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.token)

	if len(cmd.token) != 46 || !strings.HasPrefix(cmd.token, "Qm") || !is_alphanumeric {
		cmd.log.Error("Invalid token")
		return
	}

	br, err := cmd.c.FaucetTokenCheck(cmd.token, cmd.did)
	if err != nil {
		cmd.log.Info("Cannot get token details")
		return
	}
	fmt.Println(br.Message)

	cmd.log.Info("Validated token details successfully")
}

func (cmd *Command) TransferRBT() {
	if cmd.senderAddr == "" {
		cmd.log.Info("Sender address cannot be empty")
		fmt.Print("Enter Sender DID : ")
		_, err := fmt.Scan(&cmd.senderAddr)
		if err != nil {
			cmd.log.Error("Failed to get Sender DID")
			return
		}
	}
	_, senderDID, ok := util.ParseAddress(cmd.senderAddr)
	if !ok {
		cmd.log.Error("Invalid sender address")
	}
	cmd.senderAddr = senderDID

	if cmd.receiverAddr == "" {
		cmd.log.Info("Receiver address cannot be empty")
		fmt.Print("Enter Receiver DID : ")
		_, err := fmt.Scan(&cmd.receiverAddr)
		if err != nil {
			cmd.log.Error("Failed to get Receiver DID")
			return
		}
	}

	_, reciverDID, ok := util.ParseAddress(cmd.receiverAddr)
	if !ok {
		cmd.log.Error("Invalid reciver address")
	}
	cmd.receiverAddr = reciverDID

	isAlphanumericSender := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.senderAddr)
	isAlphanumericReceiver := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.receiverAddr)
	if !isAlphanumericSender || !isAlphanumericReceiver {
		cmd.log.Error("Invalid sender or receiver address. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.senderAddr, "bafybmi") || len(cmd.senderAddr) != 59 || !strings.HasPrefix(cmd.receiverAddr, "bafybmi") || len(cmd.receiverAddr) != 59 {
		cmd.log.Error("Invalid sender or receiver DID")
		return
	}
	if cmd.rbtAmount < 0.001 {
		cmd.log.Error("Invalid RBT amount. RBT amount should be atlease 0.001")
		return
	}
	rbtTransferReq := models.TransactionRequest{
		Initiator: cmd.senderAddr,
		Owner:     cmd.receiverAddr,
		Tokens: models.TransactionTokenDetails{
			RBT: cmd.rbtAmount,
		},
		Memo: cmd.transComment,
	}

	br, err := cmd.c.TransferRBT(&rbtTransferReq)
	if err != nil {
		cmd.log.Error("Failed RBT transfer", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to trasnfer RBT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("RBT transferred successfully")
}
