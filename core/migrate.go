package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/contract"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	didm "github.com/rubixchain/rubixgoplatform/did"
	"github.com/rubixchain/rubixgoplatform/token"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	BatchSize int = 100
)

type MigrateRequest struct {
	DIDType   int    `json:"did_type"`
	PrivPWD   string `json:"priv_pwd"`
	QuorumPWD string `json:"quorum_pwd"`
}

type DIDJson struct {
	PeerID string `json:"peerid"`
	DID    string `json:"didHash"`
	Wallet string `json:"walletHash"`
}

type MigrationState struct {
	DID            string            `json:"did"`
	Index          int               `json:"index"`
	MIndex         int               `json:"mindex"`
	Migration      bool              `json:"migration"`
	MigrationDone  bool              `json:"migration_done"`
	InvalidTokens  []string          `json:"invalid_tokens"`
	MigrateTokens  []string          `json:"migrate_tokens"`
	MigrateDetials map[string]string `json:"migrate_detials"`
	MigrateMap     map[string]string `json:"migrate_map"`
	InvalidMap     map[string]bool   `json:"invalid_map"`
	DiscardTokens  []string          `json:"discard_tokens"`
}

func (c *Core) removeDIDMap(peerID string) {
	// curl --location --request DELETE '13.76.134.226:9090/remove/<did>'
	ec, err := ensweb.NewClient(&config.Config{ServerAddress: "13.76.134.226", ServerPort: "9090", Production: "false"}, c.log)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	req, err := ec.JSONRequest("DELETE", "/remove/"+peerID, nil)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	resp, err := ec.Do(req)
	if err != nil {
		c.log.Error("Failed to remove old did map", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.log.Error("Failed to remove old did map", "status", resp.StatusCode)
		return
	}
	c.log.Info("Removed old did & peer map")
}

func (c *Core) IsArbitaryMode() bool {
	return c.arbitaryMode
}

func (c *Core) LockTokens(ts []string) *model.BasicResponse {
	br := &model.BasicResponse{
		Status: false,
	}
	err := c.srv.AddLockedTokens(ts)
	if err != nil {
		br.Message = "Failed to lock tokens, " + err.Error()
		return br
	}
	br.Status = true
	br.Message = "All tokens are locked successfully"
	return br
}

func (c *Core) MigrateNode(reqID string, m *MigrateRequest, didDir string) {
	err := c.migrateNode(reqID, m, didDir)
	br := model.BasicResponse{
		Status:  true,
		Message: "DID migrated successfully",
	}
	if err != nil {
		br.Status = false
		br.Message = err.Error()
	}
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("Failed to get did channels")
		return
	}
	dc.OutChan <- &br
}

func (c *Core) cleanMirgate(did string, clean *bool) {
	if *clean {
		c.w.ClearTokens(did)
		c.w.ClearTokenBlocks(token.RBTTokenType)
	}
}

func (c *Core) migrateNode(reqID string, m *MigrateRequest, didDir string) error {
	rubixDir := os.Getenv("HOME") + "/Rubix/"
	if runtime.GOOS == "windows" {
		rubixDir = "C:/Rubix/"
	}
	rb, err := ioutil.ReadFile(rubixDir + "DATA/DID.json")
	if err != nil {
		c.log.Error("Failed to migrate, invalid file", "err", err)
		return fmt.Errorf("unable to find DID.json file")
	}
	var d []DIDJson
	err = json.Unmarshal(rb, &d)
	if err != nil {
		c.log.Error("Failed to migrate, invalid parsing", "err", err)
		return fmt.Errorf("invalid DID.json file, unable to parse")
	}
	c.log.Info("Node DID: " + d[0].DID)
	resume := false
	var md MigrationState
	did := ""
	if util.IsFileExist(d[0].DID + ".json") {
		resume = true
		rb, err := ioutil.ReadFile(d[0].DID + ".json")
		if err != nil {
			c.log.Error("failed to resume state, failed to read did file", "err", err)
			return fmt.Errorf("failed to resume state, failed to read did file")
		}
		err = json.Unmarshal(rb, &md)
		if err != nil {
			c.log.Error("failed to resume state, failed to parse file", "err", err)
			return fmt.Errorf("failed to resume state, failed to parse file")
		}
		did = md.DID
	}
	if !resume {
		didCreate := didm.DIDCreate{
			Dir:            didDir,
			Type:           m.DIDType,
			PrivPWD:        m.PrivPWD,
			QuorumPWD:      m.QuorumPWD,
			DIDImgFileName: rubixDir + "DATA/" + d[0].DID + "/DID.png",
			PubImgFile:     rubixDir + "DATA/" + d[0].DID + "/PublicShare.png",
			PrivImgFile:    rubixDir + "DATA/" + d[0].DID + "/PrivateShare.png",
		}

		_, err = os.Stat(didCreate.DIDImgFileName)
		if err != nil {
			c.log.Error("Failed to migrate, missing DID.png file", "err", err)
			return fmt.Errorf("failed to migrate, missing DID.png file")
		}
		_, err = os.Stat(didCreate.PubImgFile)
		if err != nil {
			c.log.Error("Failed to migrate, missing PublicShare.png file", "err", err)
			return fmt.Errorf("failed to migrate, missing PublicShare.png file")
		}
		did, err = c.d.MigrateDID(&didCreate)
		if err != nil {
			c.log.Error("Failed to migrate, failed in creation of new DID address", "err", err, "msg", did)
			return fmt.Errorf("failed to migrate, failed in creation of new DID address")
		}

		dt := wallet.DIDType{
			DID:    did,
			DIDDir: didCreate.Dir,
			Type:   didCreate.Type,
			Config: didCreate.Config,
		}
		//clean := true
		//defer c.cleanMirgate(did, &clean)

		err = c.w.CreateDID(&dt)
		if err != nil {
			c.log.Error("Failed to create did in the wallet", "err", err)
			return fmt.Errorf("failed to create did in the wallet")
		}
	}

	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		c.log.Error("Failed to setup did crypto", "err", err)
		return fmt.Errorf("failed to setup did crypto")
	}

	tokens, err := util.GetAllFiles(rubixDir + "Wallet/TOKENS/")
	if err != nil {
		c.log.Error("Failed to migrate, failed to read token files", "err", err)
		return fmt.Errorf("failed to migrate, failed to read token files")
	}
	h := sha256.New()
	h.Write([]byte(d[0].DID))
	ha := h.Sum(nil)
	addr := int(ha[0]) % len(c.arbitaryAddr)
	c.log.Info("Conneting to node : " + c.arbitaryAddr[addr])
	p, err := c.getPeer(c.arbitaryAddr[addr])
	if err != nil {
		c.log.Error("Failed to migrate, failed to connect arbitary peer", "err", err, "peer", c.arbitaryAddr[addr])
		return fmt.Errorf("failed to migrate, failed to connect arbitary peer")
	}
	defer p.Close()
	if !c.checkDIDMigrated(p, d[0].DID) {
		c.log.Error("Failed to migrate, unable to migrate did")
		return fmt.Errorf("failed to migrate, unable to migrate did")
	}
	st := time.Now()
	numTokens := len(tokens)
	index := 0
	mindex := 0
	finishCount := 0
	migration := false
	migrationDone := false
	invalidTokens := make([]string, 0)
	migrateTokens := make([]string, 0)
	migrateDetials := make(map[string]string)
	migratedMap := make(map[string]string)
	invalidMap := make(map[string]bool)
	discardTokens := make([]string, 0)
	if resume {
		index = md.Index
		mindex = md.MIndex
		migration = md.Migration
		migrationDone = md.MigrationDone
		if len(md.InvalidTokens) > 0 {
			invalidTokens = append(invalidTokens, md.InvalidTokens...)
		}
		if len(md.MigrateTokens) > 0 {
			migrateTokens = append(migrateTokens, md.MigrateTokens...)
		}
		if len(md.DiscardTokens) > 0 {
			discardTokens = append(discardTokens, md.DiscardTokens...)
		}
		for k, v := range md.MigrateDetials {
			migrateDetials[k] = v
		}
		for k, v := range md.MigrateMap {
			migratedMap[k] = v
		}
		for k, v := range md.InvalidMap {
			invalidMap[k] = v
		}
	}
	if !migrationDone {
		for {
			tis := make([]contract.TokenInfo, 0)
			gtis := make([]block.GenesisTokenInfo, 0)
			tts := make([]block.TransTokens, 0)
			batchIndex := 0
			stime := time.Now()
			c.log.Info("Starting the batch")
			tls := make([]int, 0)
			var tns []int
			thashes := make([]string, 0)
			tkns := make([]string, 0)
			if migration {
				tt := make([]string, 0)
				for {
					t := migrateTokens[mindex+batchIndex]
					discard := false
					for i := range discardTokens {
						if discardTokens[i] == t {
							discard = true
							break
						}
					}
					if !discard {
						tk, err := ioutil.ReadFile(rubixDir + "Wallet/TOKENS/" + t)
						if err != nil {
							c.log.Error("Failed to migrate, failed to read token files", "err", err)
							return fmt.Errorf("failed to migrate, failed to read token files")
						}
						tl, thash, _, _ := token.GetWholeTokenValue(string(tk))
						thashes = append(thashes, thash)
						tls = append(tls, tl)
						tt = append(tt, t)
					}
					batchIndex++
					if mindex+batchIndex == len(migrateTokens) || batchIndex == BatchSize {
						break
					}
				}
				if len(thashes) > 0 {
					var br model.TokenNumberResponse
					err = p.SendJSONRequest("POST", APIGetTokenNumber, nil, thashes, &br, true, time.Minute*10)
					if err != nil {
						c.log.Error("Failed to migrate, failed to get token number", "err", err)
						return fmt.Errorf("failed to migrate, failed to get token number")
					}
					if !br.Status {
						c.log.Error("Failed to migrate, failed to get token number", "msg", br.Message)
						return fmt.Errorf("failed to migrate, failed to get token number")
					}
					tns = br.TokenNumbers
					if len(tns) != len(thashes) {
						c.log.Error("Failed to migrate, failed to get token number properly")
						return fmt.Errorf("failed to migrate, failed to get token number properly")
					}
					for i := range tns {
						ntd := token.GetTokenString(tls[i], tns[i])
						tb := bytes.NewReader([]byte(ntd))
						tid, err := c.ipfs.Add(tb)
						if err != nil {
							c.log.Error("Failed to migrate, failed to add token file", "err", err)
							return fmt.Errorf("failed to migrate, failed to add token file")
						}
						migrateDetials[tid] = tt[i] + "," + ntd
						migratedMap[tt[i]] = tid
						tkns = append(tkns, tid)
					}
				}
				mindex = mindex + batchIndex
				if mindex == len(migrateTokens) {
					migrationDone = true
				}
			} else {
				for {
					t := tokens[index]
					discard := false
					for i := range discardTokens {
						if discardTokens[i] == t {
							c.log.Info("Discarding token", "token", t)
							discard = true
							break
						}
					}
					if !discard {
						tk, err := ioutil.ReadFile(rubixDir + "Wallet/TOKENS/" + t)
						if err != nil {
							c.log.Error("Failed to migrate, failed to read token files", "err", err)
							return fmt.Errorf("failed to migrate, failed to read token files")
						}
						tl, thash, needMigration, err := token.GetWholeTokenValue(string(tk))
						if err != nil {
							//c.log.Info("Invalid token skipping : " + t)
							invalidTokens = append(invalidTokens, t)
							invalidMap[t] = true
							index++
							if index == numTokens {
								break
							}
							continue
						} else if needMigration {
							//c.log.Info("Token need migration : " + t)
							migrateTokens = append(migrateTokens, t)
							index++
							if index == numTokens {
								break
							}
							continue
						}
						tb := bytes.NewReader(tk)
						tid, err := c.ipfs.Add(tb)
						//tid, err := c.ipfs.Add(tb, ipfsnode.Pin(false), ipfsnode.OnlyHash(true))
						if err != nil {
							c.log.Error("Failed to migrate, failed to add token file", "err", err)
							return fmt.Errorf("failed to migrate, failed to add token file")
						}
						if t != tid {
							c.ipfs.Unpin(tid)
							//c.log.Info("Token hash not matching Invalid token skipping : " + t)
							invalidTokens = append(invalidTokens, t)
							invalidMap[t] = true
							index++
							if index == numTokens {
								break
							}
							continue
						}
						tls = append(tls, tl)
						thashes = append(thashes, thash)
						tkns = append(tkns, t)
					}
					index++
					batchIndex++
					if batchIndex == BatchSize || index == numTokens {
						break
					}
				}
			}
			if !migration {
				var br model.TokenNumberResponse
				err = p.SendJSONRequest("POST", APIGetTokenNumber, nil, thashes, &br, true, time.Minute*10)
				if err != nil {
					c.log.Error("Failed to migrate, failed to get token number", "err", err)
					return fmt.Errorf("failed to migrate, failed to get token number")
				}
				if !br.Status {
					c.log.Error("Failed to migrate, failed to get token number", "msg", br.Message)
					return fmt.Errorf("failed to migrate, failed to get token number")
				}
				tns = br.TokenNumbers
				if len(tns) != len(tls) {
					c.log.Error("Failed to migrate, failed to get token number properly")
					return fmt.Errorf("failed to migrate, failed to get token number properly")
				}
			}
			if len(tkns) > 0 {
				for i, t := range tkns {
					tn := tns[i]
					tl := tls[i]
					if !token.ValidateTokenDetials(tl, tn) {
						c.ipfs.Unpin(t)
						//c.log.Info("Invalid token skipping : " + t)
						invalidTokens = append(invalidTokens, t)
						invalidMap[t] = true
						continue
					}

					tk := ""
					if migration {
						dt := strings.Split(migrateDetials[t], ",")
						tk = dt[0]
					} else {
						tk = t
					}
					fb, err := os.Open(rubixDir + "Wallet/TOKENCHAINS/" + tk + ".json")
					if err != nil {
						c.log.Error("Failed to migrate, failed to read token chain files", "err", err)
						return fmt.Errorf("failed to migrate, failed to read token chain files")
					}
					tcid, err := c.ipfs.Add(fb)
					if err != nil {
						c.log.Error("Failed to migrate, failed to add token chain file", "err", err)
						return fmt.Errorf("failed to migrate, failed to add token chain file")
					}

					gti := block.GenesisTokenInfo{
						Token:           t,
						TokenLevel:      tl,
						TokenNumber:     tn,
						MigratedBlockID: tcid,
					}
					if migration {
						gti.PreviousID = tk
					}
					ti := contract.TokenInfo{
						Token:     t,
						TokenType: token.RBTTokenType,
						OwnerDID:  did,
					}
					tt := block.TransTokens{
						Token:     t,
						TokenType: token.RBTTokenType,
					}
					gtis = append(gtis, gti)
					tis = append(tis, ti)
					tts = append(tts, tt)
				}
				if len(gtis) > 0 {
					/* etime := time.Now()
					dtime := etime.Sub(stime)
					c.log.Info("Starting the signature", "duration", dtime)
					stime = time.Now() */
					ts := &contract.TransInfo{
						Comment:     "Migrating Token at : " + time.Now().String(),
						TransTokens: tis,
					}
					st := &contract.ContractType{
						Type:      contract.SCDIDMigrateType,
						TransInfo: ts,
					}
					sc := contract.CreateNewContract(st)
					err = sc.UpdateSignature(dc)
					if err != nil {
						c.log.Error("Failed to migrate, failed to update signature", "err", err)
						return fmt.Errorf("failed to migrate, failed to update signature")
					}
					/* dtime = etime.Sub(stime)
					c.log.Info("Signature done", "duration", dtime) */
					gb := &block.GenesisBlock{
						Type: block.TokenMigratedType,
						Info: gtis,
					}
					ctcb := make(map[string]*block.Block)
					ntcb := &block.TokenChainBlock{
						TransactionType: block.TokenMigratedType,
						TokenOwner:      did,
						GenesisBlock:    gb,
						SmartContract:   sc.GetBlock(),
						TransInfo: &block.TransInfo{
							Tokens: tts,
						},
					}
					//ctcb := make
					blk := block.CreateNewBlock(ctcb, ntcb)
					if blk == nil {
						c.log.Error("Failed to migrate, failed to create new token chain block")
						return fmt.Errorf("failed to migrate, failed to create new token chain block")
					}
					sr := &SignatureRequest{
						TokenChainBlock: blk.GetBlock(),
					}
					sig, ok := c.getArbitrationSignature(p, sr)
					if !ok {
						if sig == "" {
							c.log.Error("Failed to migrate, failed to get signature")
							return fmt.Errorf("failed to migrate, failed to get signature")
						} else {
							msgs := strings.Split(sig, ",")
							for i, str := range msgs {
								if i != 0 {
									if migration {
										dt := strings.Split(migrateDetials[str], ",")
										str = dt[0]
									}
									c.log.Error("Token already migrated, discarding the token", "token", str)
									discardTokens = append(discardTokens, str)
								}
							}
							if migration {
								mindex = mindex - len(tkns)
							} else {
								index = index - len(tkns)
							}
							continue
						}
					}
					err = blk.ReplaceSignature(p.GetPeerDID(), sig)
					if err != nil {
						c.log.Error("Failed to migrate, failed to update arbitary signature")
						return fmt.Errorf("failed to migrate, failed to update arbitary signature")
					}
					err = c.w.CreateTokenBlock(blk)
					if err != nil {
						c.log.Error("Failed to migrate, failed to add token chain block", "err", err)
						return fmt.Errorf("failed to migrate, failed to add token chain block")
					}
					for _, ti := range tis {
						t := ti.Token
						tkn := &wallet.Token{
							TokenID:     t,
							DID:         did,
							TokenValue:  1,
							TokenStatus: wallet.TokenIsFree,
						}
						err = c.w.CreateToken(tkn)
						if err != nil {
							c.log.Error("Failed to migrate, failed to add token to wallet", "err", err)
							return fmt.Errorf("failed to migrate, failed to add token to wallet")
						}
					}
					discardTokens = make([]string, 0)
					finishCount = finishCount + len(tkns)
					c.log.Info("Number of tokens migrtaed", "count", finishCount)
					etime := time.Now()
					dtime := etime.Sub(stime)
					c.log.Info("Batch process end", "duration", dtime)
				}
			}
			if migration {
				if migrationDone {
					break
				}
			} else if index >= numTokens {
				if len(migrateTokens) > 0 {
					c.log.Info("Started migration token", "num_tokens", len(migrateTokens))
					fp, err := os.Create("migratedtokens.txt")
					if err == nil {
						for i := range migrateTokens {
							fp.WriteString(migrateTokens[i])
						}
						fp.Close()
					}
					migration = true
				} else {
					migrationDone = true
					break
				}
			}
			md = MigrationState{
				DID:            did,
				Index:          index,
				MIndex:         mindex,
				Migration:      migration,
				MigrationDone:  migrationDone,
				InvalidTokens:  invalidTokens,
				MigrateTokens:  migrateTokens,
				MigrateDetials: migrateDetials,
				MigrateMap:     migratedMap,
				InvalidMap:     invalidMap,
				DiscardTokens:  discardTokens,
			}
			jb, err := json.Marshal(md)
			if err != nil {
				c.log.Error("failed to migrate, failed to marshal migration state", "err", err)
				return fmt.Errorf("failed to migrate, failed to marshal migration state")
			}
			fp, err := os.Create(d[0].DID + ".json")
			if err == nil {
				fp.Write(jb)
				fp.Close()
			}
		}
	}
	md = MigrationState{
		DID:            did,
		Index:          index,
		MIndex:         mindex,
		Migration:      migration,
		MigrationDone:  migrationDone,
		InvalidTokens:  invalidTokens,
		MigrateTokens:  migrateTokens,
		MigrateDetials: migrateDetials,
		MigrateMap:     migratedMap,
		InvalidMap:     invalidMap,
		DiscardTokens:  discardTokens,
	}
	jb, err := json.Marshal(md)
	if err != nil {
		c.log.Error("failed to migrate, failed to marshal migration state", "err", err)
		return fmt.Errorf("failed to migrate, failed to marshal migration state")
	}
	fp, err := os.Create(d[0].DID + ".json")
	if err == nil {
		fp.Write(jb)
		fp.Close()
	}
	if len(invalidTokens) > 0 {
		fp, err := os.Create("invalidtokens.txt")
		if err == nil {
			for i := range invalidTokens {
				fp.WriteString(invalidTokens[i])
			}
			fp.Close()
		}
	}
	// if len(migrateTokens) > 0 {
	// 	fp, err := os.Open("migratedtokens.txt")
	// 	if err == nil {
	// 		for i := range migrateTokens {
	// 			fp.WriteString(migrateTokens[i])
	// 		}
	// 		fp.Close()
	// 	}
	// }
	creditFiles, err := util.GetAllFiles(rubixDir + "Wallet/WALLET_DATA/Credits/")
	if err != nil {
		c.log.Error("Failed to migrate, failed to read credit files", "err", err)
		return fmt.Errorf("failed to migrate, failed to credit token files")
	}
	for _, cf := range creditFiles {
		cb, err := ioutil.ReadFile(rubixDir + "Wallet/WALLET_DATA/Credits/" + cf)
		if err != nil {
			c.log.Error("Failed to migrate, failed to read credit file", "err", err)
			return fmt.Errorf("failed to migrate, failed to credit token file")
		}
		var cs []CreditSignature
		err = json.Unmarshal(cb, &cs)
		if err != nil {
			c.log.Error("Failed to migrate, failed to parse credit file", "err", err)
			return fmt.Errorf("failed to migrate, failed to parse credit file")
		}
		var ncs CreditScore
		ncs.Credit = make([]CreditSignature, 0)
		for _, s := range cs {
			sig := util.ConvertBitString(s.Signature)
			if sig == nil {
				c.log.Error("Failed to migrate, failed to parse credit signature")
				return fmt.Errorf("failed to migrate, failed to parse credit signature")
			}
			ns := CreditSignature{
				DID:       s.DID,
				Hash:      s.Hash,
				Signature: util.HexToStr(sig),
			}
			ncs.Credit = append(ncs.Credit, ns)
		}
		if len(ncs.Credit) > 0 {
			jb, err := json.Marshal(&ncs)
			if err != nil {
				c.log.Error("Failed to migrate, failed to marshal credit", "err", err)
				return fmt.Errorf("failed to migrate, failed to marshal credit")
			}
			err = c.w.StoreCredit(did, base64.StdEncoding.EncodeToString(jb))
			if err != nil {
				c.log.Error("Failed to migrate, failed to store credit", "err", err)
				return fmt.Errorf("failed to migrate, failed to store credit")
			}
		}
	}
	if !c.mapMigratedDID(p, d[0].DID, did) {
		c.log.Error("Failed to migrate, failed to store credit", "err", err)
		return fmt.Errorf("failed to migrate, failed to store credit")
	}
	mt := false
	if len(migrateTokens) > 0 {
		mt = true
	}
	lt := false
	if len(invalidMap) > 0 {
		lt = true
	}
	for i := 0; i < numTokens; i++ {
		t := tokens[i]
		if mt {
			tid, ok := migratedMap[t]
			if ok {
				t = tid
			}
		}
		if lt {
			s, ok := invalidMap[t]
			if ok && s {
				continue
			}
		}
		ok, err := c.w.Pin(t, wallet.OwnerRole, did, "Migrated Token", "NA", "NA", float64(1))
		if err != nil || !ok {
			c.log.Error("Failed to migrate, failed to pin token", "err", err)
			return fmt.Errorf("failed to migrate, failed to pin token")
		}
	}
	et := time.Now()
	dif := et.Sub(st)
	c.log.Info("Tokens signatures completed", "duration", dif)
	st = time.Now()
	c.removeDIDMap(d[0].PeerID)
	c.ec.ExplorerMapDID(d[0].DID, did, c.peerID)
	dif = et.Sub(st)
	c.log.Info("Tokens migration completed", "duration", dif)
	c.log.Info(fmt.Sprintf("Old DID=%s migrated to New DID=%s", d[0].DID, did))
	c.log.Info(fmt.Sprintf("Number of tokens migrated =%d", len(tokens)))
	c.log.Info(fmt.Sprintf("Number of credits migrated =%d", len(creditFiles)))
	c.log.Info("Migration done successfully")
	return nil
}
