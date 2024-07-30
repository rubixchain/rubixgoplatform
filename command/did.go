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

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/nlss"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/spf13/cobra"
)

func didCommandGroup(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "did",
		Short: "DID related subcommands",
		Long:  "DID related subcommands",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		createDID(cmdCfg),
		listDIDs(cmdCfg),
		registerDID(cmdCfg),
		accountInfoCmd(cmdCfg),
	)

	return cmd
}

func createDID(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a DID",
		Long:  "Create a DID",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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
			if cmdCfg.didType == did.LiteDIDMode {
				if cmdCfg.privKeyFile == "" || cmdCfg.pubKeyFile == "" {
					cmdCfg.log.Error("private key & public key file names required")
					return nil
				}
			} else if cmdCfg.didType == did.WalletDIDMode {
				f, err := os.Open(cmdCfg.imgFile)
				if err != nil {
					cmdCfg.log.Error("failed to open image", "err", err)
					return nil
				}
				defer f.Close()
				img, _, err := image.Decode(f)
				if err != nil {
					cmdCfg.log.Error("failed to decode image", "err", err)
					return nil
				}
				bounds := img.Bounds()
				w, h := bounds.Max.X, bounds.Max.Y
		
				if w != 256 || h != 256 {
					cmdCfg.log.Error("invalid image size", "err", err)
					return nil
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
				message := cmdCfg.didSecret + util.GetMACAddress()
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
		
				err = util.CreatePNGImage(outPixels, w, h, cmdCfg.didImgFile)
				if err != nil {
					cmdCfg.log.Error("failed to create image", "err", err)
					return nil
				}
				pvtShare := make([]byte, 0)
				pubShare := make([]byte, 0)
				numBytes := len(outPixels)
				for i := 0; i < numBytes; i = i + 1024 {
					pvS, pbS := nlss.Gen2Shares(outPixels[i : i+1024])
					pvtShare = append(pvtShare, pvS...)
					pubShare = append(pubShare, pbS...)
				}
				err = util.CreatePNGImage(pvtShare, w*4, h*2, cmdCfg.privImgFile)
				if err != nil {
					cmdCfg.log.Error("failed to create image", "err", err)
					return nil
				}
				err = util.CreatePNGImage(pubShare, w*4, h*2, cmdCfg.pubImgFile)
				if err != nil {
					cmdCfg.log.Error("failed to create image", "err", err)
					return nil
				}
			}
			if cmdCfg.didType != did.BasicDIDMode && cmdCfg.didType != did.LiteDIDMode {
				if cmdCfg.privKeyFile == "" || cmdCfg.pubKeyFile == "" {
					cmdCfg.log.Error("private key & public key file names required")
					return nil
				}
				pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: cmdCfg.privPWD})
				if err != nil {
					cmdCfg.log.Error("failed to create keypair", "err", err)
					return nil
				}
				err = util.FileWrite(cmdCfg.privKeyFile, pvtKey)
				if err != nil {
					cmdCfg.log.Error("failed to write private key file", "err", err)
					return nil
				}
				err = util.FileWrite(cmdCfg.pubKeyFile, pubKey)
				if err != nil {
					cmdCfg.log.Error("failed to write public key file", "err", err)
					return nil
				}
			}
			cfg := did.DIDCreate{
				Type:           cmdCfg.didType,
				Secret:         cmdCfg.didSecret,
				RootDID:        cmdCfg.didRoot,
				PrivPWD:        cmdCfg.privPWD,
				QuorumPWD:      cmdCfg.quorumPWD,
				ImgFile:        cmdCfg.imgFile,
				DIDImgFileName: cmdCfg.didImgFile,
				PubImgFile:     cmdCfg.pubImgFile,
				PubKeyFile:     cmdCfg.pubKeyFile,
				MnemonicFile:   cmdCfg.mnemonicFile,
				ChildPath:      cmdCfg.ChildPath,
			}
			msg, status := cmdCfg.c.CreateDID(&cfg)
			if !status {
				cmdCfg.log.Error("Failed to create DID", "message", msg)
				return nil
			}
			cmdCfg.log.Info(fmt.Sprintf("DID %v created successfully", msg))
			return nil
		},
	}

	cmd.Flags().BoolVar(&cmdCfg.forcePWD, "fp", false, "Force password entry")
	cmd.Flags().StringVar(&cmdCfg.privPWD, "privPWD", "mypassword", "Private key password")
	cmd.Flags().StringVar(&cmdCfg.quorumPWD, "quorumPWD", "mypassword", "Quorum key password")
	cmd.Flags().IntVar(&cmdCfg.didType, "didType", 4, "DID Creation type")
	cmd.Flags().StringVar(&cmdCfg.imgFile, "imgFile", did.ImgFileName, "DID creation image")
	cmd.Flags().StringVar(&cmdCfg.privKeyFile, "privKeyFile", did.PvtKeyFileName, "Private key file")
	cmd.Flags().StringVar(&cmdCfg.pubKeyFile, "pubKeyFile", did.PubKeyFileName, "Public key file")
	cmd.Flags().StringVar(&cmdCfg.mnemonicFile, "mnemonicKeyFile", did.MnemonicFileName, "Mnemonic key file")
	cmd.Flags().IntVar(&cmdCfg.ChildPath, "ChildPath", 0, "BIP child Path")

	return cmd
}

func listDIDs(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Fetch every DID present in the node",
		Long:  "Fetch every DID present in the node",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			response, err := cmdCfg.c.GetAllDIDs()
			if err != nil {
				cmdCfg.log.Error("Invalid response from the node", "err", err)
				return err
			}
			if !response.Status {
				cmdCfg.log.Error("Failed to get DIDs", "message", response.Message)
				return err
			}
			for i := range response.AccountInfo {
				fmt.Printf("Address : %s\n", response.AccountInfo[i].DID)
			}
			cmdCfg.log.Info("Got all DID successfully")

			return nil
		},
	}

	return cmd
}

func registerDID(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register DID",
		Long:  "Register DID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Info("DID cannot be empty")
				fmt.Print("Enter DID : ")
				_, err := fmt.Scan(&cmdCfg.did)
				if err != nil {
					cmdCfg.log.Error("Failed to get DID")
					return nil
				}
			}
			is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.did)
			if !strings.HasPrefix(cmdCfg.did, "bafybmi") || len(cmdCfg.did) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid DID")
				return nil
			}

			br, err := cmdCfg.c.RegisterDID(cmdCfg.did)

			if err != nil {
				errMsg := fmt.Errorf("failed to register DID, err: %v", err)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}

			if !br.Status {
				errMsg := fmt.Errorf("failed to register DID, %v", br.Message)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}

			msg, status := signatureResponse(cmdCfg, br)

			if !status {
				errMsg := fmt.Errorf("failed to register DID, %v", msg)
				cmdCfg.log.Error(errMsg.Error())
				return errMsg
			}
			cmdCfg.log.Info("DID registered successfully")

			return nil
		},
	}

	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}

func accountInfoCmd(cmdCfg *CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Get the account balance information of a DID",
		Long:  "Get the account balance information of a DID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmdCfg.did == "" {
				cmdCfg.log.Info("DID cannot be empty")
				fmt.Print("Enter DID : ")
				_, err := fmt.Scan(&cmdCfg.did)
				if err != nil {
					cmdCfg.log.Error("Failed to get DID")
					return nil
				}
			}
			is_alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(cmdCfg.did)
			if !strings.HasPrefix(cmdCfg.did, "bafybmi") || len(cmdCfg.did) != 59 || !is_alphanumeric {
				cmdCfg.log.Error("Invalid DID")
				return nil 
			}

			info, err := cmdCfg.c.GetAccountInfo(cmdCfg.did)
			if err != nil {
				cmdCfg.log.Error("Invalid response from the node", "err", err)
				return nil
			}
			fmt.Printf("Response : %v\n", info)
			if !info.Status {
				cmdCfg.log.Error("Failed to get account info", "message", info.Message)
				return nil
			} else {
				cmdCfg.log.Info("Successfully got the account balance information")
				fmt.Printf("RBT : %10.5f, Locked RBT : %10.5f, Pledged RBT : %10.5f\n", info.AccountInfo[0].RBTAmount, info.AccountInfo[0].LockedRBT, info.AccountInfo[0].PledgedRBT)
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&cmdCfg.did, "did", "", "DID")

	return cmd
}

func signatureResponse(cmdCfg *CommandConfig, br *model.BasicResponse, timeout ...time.Duration) (string, bool) {
	pwdSet := false
	password := cmdCfg.privPWD
	for {
		if !br.Status {
			return br.Message, false
		}
		if br.Result == nil {
			return br.Message, true
		}
		cmdCfg.log.Info("Got the request for the signature")
		jb, err := json.Marshal(br.Result)
		if err != nil {
			return "Invalid response, " + err.Error(), false
		}
		var sr did.SignReqData
		err = json.Unmarshal(jb, &sr)
		if err != nil {
			return "Invalid response, " + err.Error(), false
		}
		if cmdCfg.forcePWD && !pwdSet {
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
		case did.LiteDIDMode:
			sresp.Password = password
		case did.BasicDIDMode:
			sresp.Password = password
		case did.StandardDIDMode:
			privKey, err := ioutil.ReadFile(cmdCfg.privKeyFile)
			if err != nil {
				return "Failed to open private key file, " + err.Error(), false
			}
			key, _, err := crypto.DecodeKeyPair(password, privKey, nil)
			if err != nil {
				return "Failed to decode private key file, " + err.Error(), false
			}
			cmdCfg.log.Info("Doing the private key signature")
			sig, err := crypto.Sign(key, sr.Hash)
			if err != nil {
				return "Failed to do signature, " + err.Error(), false
			}
			sresp.Signature.Signature = sig
		case did.WalletDIDMode:
			hash := sr.Hash
			if !sr.OnlyPrivKey {
				byteImg, err := util.GetPNGImagePixels(cmdCfg.privImgFile)
				if err != nil {
					return "Failed to read private share image file, " + err.Error(), false
				}
				cmdCfg.log.Info("Doing the private share signature")
				ps := util.ByteArraytoIntArray(byteImg)
				randPosObject := util.RandomPositions("signer", string(sr.Hash), 32, ps)

				finalPos := randPosObject.PosForSign
				pvtPos := util.GetPrivatePositions(finalPos, ps)
				pvtPosStr := util.IntArraytoStr(pvtPos)

				hashPvtSign := util.HexToStr(util.CalculateHash([]byte(pvtPosStr), "SHA3-256"))
				hash = []byte(hashPvtSign)

				bs, err := util.BitstreamToBytes(pvtPosStr)
				if err != nil {
					return "Failed to read convert bitstream, " + err.Error(), false
				}
				sresp.Signature.Pixels = bs
			}
			privKey, err := ioutil.ReadFile(cmdCfg.privKeyFile)
			if err != nil {
				return "Failed to open private key file, " + err.Error(), false
			}
			key, _, err := crypto.DecodeKeyPair(password, privKey, nil)
			if err != nil {
				return "Failed to decode private key file, " + err.Error(), false
			}
			cmdCfg.log.Info("Doing the private key signature")
			sig, err := crypto.Sign(key, hash)
			if err != nil {
				return "Failed to do signature, " + err.Error(), false
			}
			sresp.Signature.Signature = sig
		}
		br, err = cmdCfg.c.SignatureResponse(&sresp, timeout...)
		if err != nil {
			cmdCfg.log.Error("Failed to generate RBT", "err", err)
			return "Failed in signature response, " + err.Error(), false
		}
	}
}
