package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

func (cmd *Command) GenerateTestRBT() {
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

	br, err := cmd.c.GenerateTestRBT(cmd.numTokens, cmd.did)

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

func (cmd *Command) ValidateToken() {
	if cmd.token == "" {
		cmd.log.Info("Token cannot be empty")
		fmt.Print("Enter Token : ")
		_, err := fmt.Scan(&cmd.token)
		if err != nil {
			cmd.log.Error("Failed to get tokenhash")
			return
		}
	}
	br, err := cmd.c.ValidateToken(cmd.token)
	if err != nil {
		cmd.log.Error("failed to validate token", "err", err)
		return
	}

	if !br.Status {

		cmd.log.Error("failed to validate token %s", cmd.token, "msg", br.Message)
		return
	}
	cmd.log.Info("Token %s validated successfully ", cmd.token, "msg", br.Message)
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

func (cmd *Command) InitiateRBTPrePledge() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter owner DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get Sender DID")
			return
		}
	}

	isAlphanumericOwner := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !isAlphanumericOwner {
		cmd.log.Error("Invalid owner. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 {
		cmd.log.Error("Invalid owner DID")
		return
	}

	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type. TransType should be 1 or 2")
		return
	}
	rt := model.CvrAPIRequest{
		DID:        cmd.did,
		QuorumType: cmd.transType,
	}

	br, err := cmd.c.PrePledgeRBT(&rt)
	if err != nil {
		cmd.log.Error("Failed RBT pre-pledge", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to pre-pledge RBT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("RBT pre-pledged successfully")
}

func (cmd *Command) InitiateFTPrePledge() {
	if cmd.did == "" {
		cmd.log.Info("DID cannot be empty")
		fmt.Print("Enter owner DID : ")
		_, err := fmt.Scan(&cmd.did)
		if err != nil {
			cmd.log.Error("Failed to get Sender DID")
			return
		}
	}

	isAlphanumericOwner := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !isAlphanumericOwner {
		cmd.log.Error("Invalid owner. Please provide valid DID")
		return
	}
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 {
		cmd.log.Error("Invalid owner DID")
		return
	}

	if cmd.transType < 1 || cmd.transType > 2 {
		cmd.log.Error("Invalid trans type. TransType should be 1 or 2")
		return
	}
	rt := model.CvrAPIRequest{
		DID:        cmd.did,
		QuorumType: cmd.transType,
	}

	br, err := cmd.c.PrePledgeFT(&rt)
	if err != nil {
		cmd.log.Error("Failed FT pre-pledge", "err", err)
		return
	}
	msg, status := cmd.SignatureResponse(br)
	if !status {
		cmd.log.Error("Failed to pre-pledge FT", "msg", msg)
		return
	}
	cmd.log.Info(msg)
	cmd.log.Info("FT pre-pledged successfully")
}
