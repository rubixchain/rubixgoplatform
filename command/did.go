package command

import (
	"encoding/json"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
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
	if cmd.forcePWD {
		pwd, err := getpassword("Set quorum key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		npwd, err := getpassword("Re-enter quorum key password: ")
		if err != nil {
			cmd.log.Error("Failed to get password")
			return
		}
		if pwd != npwd {
			cmd.log.Error("Password mismatch")
			return
		}
		cmd.quorumPWD = pwd
	}
	if cmd.didType < 0 || cmd.didType > 4 {
		cmd.log.Error("DID Type should be between 0 and 4")
		return
	}
	if cmd.didType == did.LiteDIDMode {
		if cmd.privKeyFile == "" || cmd.pubKeyFile == "" {
			cmd.log.Error("private key & public key file names required")
			return
		}
	} else if cmd.didType == did.WalletDIDMode {
		f, err := os.Open(cmd.imgFile)
		if err != nil {
			cmd.log.Error("failed to open image", "err", err)
			return
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			cmd.log.Error("failed to decode image", "err", err)
			return
		}
		bounds := img.Bounds()
		w, h := bounds.Max.X, bounds.Max.Y

		if w != 256 || h != 256 {
			cmd.log.Error("invalid image size", "err", err)
			return
		}
		pixels := make([]byte, 0)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				pixels = append(pixels, byte(r>>8))
				pixels = append(pixels, byte(g>>8))
				pixels = append(pixels, byte(b>>8))
			}
		}
		outPixels := make([]byte, 0)
		message := cmd.didSecret + util.GetMACAddress()
		dataHash := util.CalculateHash([]byte(message), "SHA3-256")
		offset := 0
		for y := 0; y < h; y++ {
			for x := 0; x < 24; x++ {
				for i := 0; i < 32; i++ {
					outPixels = append(outPixels, dataHash[i]^pixels[offset+i])
				}
				offset = offset + 32
				dataHash = util.CalculateHash(dataHash, "SHA3-256")
			}
		}

		err = util.CreatePNGImage(outPixels, w, h, cmd.didImgFile)
		if err != nil {
			cmd.log.Error("failed to create image", "err", err)
			return
		}
		pvtShare := make([]byte, 0)
		pubShare := make([]byte, 0)
		numBytes := len(outPixels)
		for i := 0; i < numBytes; i = i + 1024 {
			pvS, pbS := nlss.Gen2Shares(outPixels[i : i+1024])
			pvtShare = append(pvtShare, pvS...)
			pubShare = append(pubShare, pbS...)
		}
		err = util.CreatePNGImage(pvtShare, w*4, h*2, cmd.privImgFile)
		if err != nil {
			cmd.log.Error("failed to create image", "err", err)
			return
		}
		err = util.CreatePNGImage(pubShare, w*4, h*2, cmd.pubImgFile)
		if err != nil {
			cmd.log.Error("failed to create image", "err", err)
			return
		}
	}
	if cmd.didType != did.BasicDIDMode && cmd.didType != did.LiteDIDMode {
		if cmd.privKeyFile == "" || cmd.pubKeyFile == "" {
			cmd.log.Error("private key & public key file names required")
			return
		}
		pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: cmd.privPWD})
		if err != nil {
			cmd.log.Error("failed to create keypair", "err", err)
			return
		}
		err = util.FileWrite(cmd.privKeyFile, pvtKey)
		if err != nil {
			cmd.log.Error("failed to write private key file", "err", err)
			return
		}
		err = util.FileWrite(cmd.pubKeyFile, pubKey)
		if err != nil {
			cmd.log.Error("failed to write public key file", "err", err)
			return
		}
	}
	cfg := did.DIDCreate{
		Type:           cmd.didType,
		Secret:         cmd.didSecret,
		RootDID:        cmd.didRoot,
		PrivPWD:        cmd.privPWD,
		QuorumPWD:      cmd.quorumPWD,
		ImgFile:        cmd.imgFile,
		DIDImgFileName: cmd.didImgFile,
		PubImgFile:     cmd.pubImgFile,
		PubKeyFile:     cmd.pubKeyFile,
		MnemonicFile:   cmd.mnemonicFile,
		ChildPath:      cmd.ChildPath,
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
	for i := range response.AccountInfo {
		fmt.Printf("Address : %s\n", response.AccountInfo[i].DID)
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

		cmd.log.Info("Got the request for the signature")

		switch res := br.Result.(type) {

		case map[string]interface{}:
			jb, err := json.Marshal(res)
			if err != nil {
				return "Invalid response, " + err.Error(), false
			}

			var sr did.SignReqData
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

			sresp := did.SignRespData{
				ID:   sr.ID,
				Mode: sr.Mode,
			}

			switch sr.Mode {
			case did.LiteDIDMode, did.BasicDIDMode:
				sresp.Password = password

			case did.StandardDIDMode:
				privKey, err := ioutil.ReadFile(cmd.privKeyFile)
				if err != nil {
					return "Failed to open private key file, " + err.Error(), false
				}
				key, _, err := crypto.DecodeKeyPair(password, privKey, nil)
				if err != nil {
					return "Failed to decode private key file, " + err.Error(), false
				}
				cmd.log.Info("Doing the private key signature")
				sig, err := crypto.Sign(key, sr.Hash)
				if err != nil {
					return "Failed to do signature, " + err.Error(), false
				}
				sresp.Signature.Signature = sig

			case did.WalletDIDMode:
				hash := sr.Hash
				if !sr.OnlyPrivKey {
					byteImg, err := util.GetPNGImagePixels(cmd.privImgFile)
					if err != nil {
						return "Failed to read private share image file, " + err.Error(), false
					}
					cmd.log.Info("Doing the private share signature")
					ps := util.ByteArraytoIntArray(byteImg)
					randPosObject := util.RandomPositions("signer", string(sr.Hash), 32, ps)

					finalPos := randPosObject.PosForSign
					pvtPos := util.GetPrivatePositions(finalPos, ps)
					pvtPosStr := util.IntArraytoStr(pvtPos)

					hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
					hash = []byte(hashPvtSign)

					bs, err := util.BitstreamToBytes(pvtPosStr)
					if err != nil {
						return "Failed to convert bitstream, " + err.Error(), false
					}
					sresp.Signature.Pixels = bs
				}

				privKey, err := ioutil.ReadFile(cmd.privKeyFile)
				if err != nil {
					return "Failed to open private key file, " + err.Error(), false
				}
				key, _, err := crypto.DecodeKeyPair(password, privKey, nil)
				if err != nil {
					return "Failed to decode private key file, " + err.Error(), false
				}
				cmd.log.Info("Doing the private key signature")
				sig, err := crypto.Sign(key, hash)
				if err != nil {
					return "Failed to do signature, " + err.Error(), false
				}
				sresp.Signature.Signature = sig
			}

			br, err = cmd.c.SignatureResponse(&sresp, timeout...)
			if err != nil {
				cmd.log.Error("Failed to generate RBT", "err", err)
				return "Failed in signature response, " + err.Error(), false
			}

		case string:
			// fallback: result is just transaction ID string
			return "Transaction is still processing, Transaction ID: " + res, true

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
		fmt.Printf("RBT : %10.*f, Locked RBT : %10.*f, Pledged RBT : %10.*f, Pinned RBT : %10.*f,  Spendable RBT : %10.*f\n,", core.MaxDecimalPlaces, info.AccountInfo[0].RBTAmount, core.MaxDecimalPlaces, info.AccountInfo[0].LockedRBT, core.MaxDecimalPlaces, info.AccountInfo[0].PledgedRBT, core.MaxDecimalPlaces, info.AccountInfo[0].PinnedRBT, core.MaxDecimalPlaces, info.AccountInfo[0].SpendableRBT)
	}
}

// CreateDIDFromPubKey request to create did from provided public key
func (cmd *Command) CreateDIDFromPubKey() {
	did, err := cmd.c.CreateDIDFromPubKey(cmd.pubKeyFile)
	if err != nil {
		cmd.log.Error("err", err)
	}
	cmd.log.Debug("received did", did)
}
