package command

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types"
)

func (cmd *Command) CreateDID() {
	if cmd.forcePWD {
		pwd, err := getpassword("Set private key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		npwd, err := getpassword("Re-enter private key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		if pwd != npwd {
			cmd.log.Error("Password mismatch")
			return
		}
		cmd.privPWD = pwd
	}

	cfg := types.DIDCreate{
		PrivPWD:   cmd.privPWD,
		Mnemonic:  cmd.mnemonic,
		ChildPath: cmd.ChildPath,
		PubKey:    cmd.pubKeyFile,
	}
	msg, status := cmd.c.CreateDID(&cfg)
	if !status {
		cmd.log.Error("Failed to create DID", "message", msg)
		return
	}
	cmd.log.Info(fmt.Sprintf("DID %v created successfully", msg))
}

func (cmd *Command) GetAllDID() {
	response, err := cmd.c.GetAllDIDs()
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	if !response.Status {
		cmd.log.Error("Failed to get DIDs", "message", response.Message)
		return
	}
	didList := response.Result.([]interface{})
	for _, did := range didList {
		fmt.Printf("Address : %s\n", did.(string))
	}
	cmd.log.Info("Got all DID successfully")
}

func (cmd *Command) RegsiterDIDCmd() {
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
	br, err := cmd.c.RegisterDID(cmd.did)

	if err != nil {
		cmd.log.Error("Failed to register DID", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to register DID", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to register DID, " + msg)
		return
	}
	cmd.log.Info("DID registered successfully")
}

func (cmd *Command) SetupDIDCmd() {
	isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmd.did)
	if !strings.HasPrefix(cmd.did, "bafybmi") || len(cmd.did) != 59 || !isAlphanumeric {
		cmd.log.Error("Invalid DID")
		return
	}
	br, err := cmd.c.RegisterDID(cmd.did)

	if err != nil {
		cmd.log.Error("Failed to register DID", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to register DID", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to register DID, " + msg)
		return
	}
	cmd.log.Info("DID registered successfully")
}

func (cmd *Command) SignatureResponse(br *model.BasicResponse, timeout ...time.Duration) (string, bool) {
	pwdSet := false
	password := cmd.privPWD

	for {
		if !br.Status {
			return br.Message, false
		}
		if br.Result == nil {
			return br.Message, true
		}

		// signature response for arbitrary signature
		if strings.Contains(br.Message, "arbitrary sign") {
			jsonbytes, err := json.Marshal(br.Result)
			if err != nil {
				errMsg := "Invalid response, " + err.Error()
				return errMsg, false
			}

			signMap := model.Signature{}
			err = json.Unmarshal(jsonbytes, &signMap)
			if err != nil {
				errMsg := "Invalid response, " + err.Error()
				return errMsg, false
			}
			return signMap.Signature, true
		}

		cmd.log.Info("Got the request for the signature")

		switch res := br.Result.(type) {

		case map[string]interface{}:
			jb, err := json.Marshal(res)
			if err != nil {
				return "Invalid response, " + err.Error(), false
			}

			var sr types.SignReqData
			err = json.Unmarshal(jb, &sr)
			if err != nil {
				return "Invalid response, " + err.Error(), false
			}

			if cmd.forcePWD && !pwdSet {
				password, err = getpassword("Enter private key password: ")
				if err != nil {
					return "Failed to get password", false
				}
				pwdSet = true
			}

			sresp := types.SignRespData{
				ID: sr.ID,
			}

			sresp.Password = password

			br, err = cmd.c.SignatureResponse(&sresp, timeout...)
			if err != nil {
				return "Failed signature response, " + err.Error(), false
			}

		case string:
			// fallback: result is just transaction ID string
			return br.Message, true

		default:
			return "Invalid response: unexpected format", false
		}
	}
}

func (cmd *Command) GetAccountInfo() {
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
	info, err := cmd.c.GetAccountInfo(cmd.did)
	if err != nil {
		cmd.log.Error("Invalid response from the node", "err", err)
		return
	}
	fmt.Printf("Response : %v\n", info)
	if !info.Status {
		cmd.log.Error("Failed to get account info", "message", info.Message)
	} else {
		cmd.log.Info("Successfully got the account information")
		fmt.Printf("RBT : %10.*f, Locked RBT : %10.*f, Pledged RBT : %10.*f, Pinned RBT : %10.*f\n", core.MaxDecimalPlaces, info.AccountInfo[0].RBTAmount, core.MaxDecimalPlaces, info.AccountInfo[0].LockedRBT, core.MaxDecimalPlaces, info.AccountInfo[0].PledgedRBT, core.MaxDecimalPlaces, info.AccountInfo[0].PinnedRBT)
	}
}

func (cmd *Command) ArbitrarySign() {
	signResp, err := cmd.c.ArbitrarySignature(cmd.signerDID, cmd.message)
	if err != nil {
		cmd.log.Error("err", err)
		return
	}
	if !signResp.Status {
		cmd.log.Error("Failed to sign, msg ", signResp.Message)
		return
	}
	msg, status := cmd.SignatureResponse(signResp)
	if !status {
		cmd.log.Error("Failed to sign, msg ", msg)
		return
	}
	var result string
	if status {
		result = fmt.Sprintf("Status : %v, Signature : %v", status, msg)
	} else {
		result = fmt.Sprintf("Status : %v, message : %v", status, msg)
	}

	cmd.log.Info(result)
}

func (cmd *Command) SignVerification() {
	result, err := cmd.c.SignVerification(cmd.signerDID, cmd.message, cmd.signature)
	if err != nil {
		cmd.log.Error("err", err)
		return
	}
	cmd.log.Info("signature verification result", result)
}

func (cmd *Command) RemoveStaleDID() {
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
	br, err := cmd.c.RemoveStaleDID(cmd.did)
	if err != nil {
		cmd.log.Error("Failed to remove DID from network", "err", err)
		return
	}

	if !br.Status {
		cmd.log.Error("Failed to remove DID from network", "msg", br.Message)
		return
	}

	msg, status := cmd.SignatureResponse(br)

	if !status {
		cmd.log.Error("Failed to remove DID from network, " + msg)
		return
	}
	cmd.log.Info("DID removed from network successfully")
}
