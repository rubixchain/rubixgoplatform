package unpledge

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"

	"golang.org/x/crypto/sha3"
)

const (
	RecordInterval int = 5000
	Difficultlevel int = 6
)

const (
	UnpledgeQueueTable string = "unpledgequeue"
)

type UnPledge struct {
	s       storage.Storage
	testNet bool
	w       *wallet.Wallet
	l       sync.Mutex
	running bool
	dir     string
	cb      UnpledgeCBType
	log     logger.Logger
}

type UnpledgeTokenList struct {
	Token string `gorm:"column:token"`
}

type UnpledgeCBType func(t string, file string) error

func InitUnPledge(s storage.Storage, w *wallet.Wallet, testNet bool, dir string, cb UnpledgeCBType, log logger.Logger) (*UnPledge, error) {
	up := &UnPledge{
		s:       s,
		testNet: testNet,
		w:       w,
		dir:     dir,
		cb:      cb,
		log:     log.Named("unpledge"),
	}
	err := up.s.Init(UnpledgeQueueTable, UnpledgeTokenList{}, true)
	if err != nil {
		up.log.Error("failed to init unpledge token list table", "err", err)
		return nil, err
	}
	var list []UnpledgeTokenList
	err = up.s.Read(UnpledgeQueueTable, &list, "token != ?", "")
	if err != nil {
		tks, err := up.w.GetAllPledgedTokens()
		if err == nil {
			list = make([]UnpledgeTokenList, 0)
			for i := range tks {
				l := UnpledgeTokenList{
					Token: tks[i].TokenID,
				}
				list = append(list, l)
			}
			for i := range list {
				err = up.s.Write(UnpledgeQueueTable, &list[i])
				if err != nil {
					up.log.Error("Failed to write unpledge list", "err", err)
					return nil, err
				}
			}
		}
	}
	//go up.runUnpledge()
	return up, nil
}

func (up *UnPledge) RunUnpledge() {
	go up.runUnpledge()
}

func sha2Hash256(input string) string {
	hash := sha256.Sum256([]byte(input))
	hashedInput := fmt.Sprintf("%x", hash)
	return hashedInput
}

func sha3Hash256(input string) string {
	hash := sha3.Sum256([]byte(input))
	hashedInput := fmt.Sprintf("%x", hash)
	return hashedInput
}

func sha3Hash256Loop(input string) string {
	var hashedInput string
	for i := 0; i < RecordInterval; i++ {
		hash := sha3.Sum256([]byte(input))
		hashedInput = fmt.Sprintf("%x", hash)
		input = hashedInput
	}
	return hashedInput
}

func (up *UnPledge) isRunning() bool {
	up.l.Lock()
	s := up.running
	up.l.Unlock()
	return s
}

func (up *UnPledge) AddUnPledge(t string) {
	var list UnpledgeTokenList
	err := up.s.Read(UnpledgeQueueTable, &list, "token = ?", t)
	if err == nil {
		up.log.Error("Token already in the unpledge list")
		return
	}
	list.Token = t
	err = up.s.Write(UnpledgeQueueTable, &list)
	if err != nil {
		up.log.Error("Error adding token "+t+" to unpledge list", "err", err)
		return
	}
	go up.runUnpledge()
}

func (up *UnPledge) runUnpledge() {
	up.l.Lock()
	if up.running {
		up.l.Unlock()
		return
	}
	up.log.Info("Unpledging started")
	up.running = true
	up.l.Unlock()
	defer func() {
		up.l.Lock()
		up.running = false
		up.l.Unlock()
	}()
	for {
		var list UnpledgeTokenList
		err := up.s.Read(UnpledgeQueueTable, &list, "token != ?", "")
		if err != nil {
			up.log.Info("All tokens are unplegded")
			break
		}
		st := time.Now()
		t := list.Token
		tt := token.RBTTokenType
		if up.testNet {
			tt = token.TestTokenType
		}
		b := up.w.GetLatestTokenBlock(t, tt)
		if b == nil {
			up.log.Error("Failed to get the latest token block, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		bid, err := b.GetBlockID(t)
		if err != nil {
			up.log.Error("Failed to get the block id, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		if b.GetTransType() != block.TokenPledgedType {
			up.log.Error("Token is not in pledged state, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		blk := b.GetTransBlock()
		if blk == nil {
			refID := b.GetRefID()
			if refID == "" {
				up.log.Error("Token block missing transaction block, removing the token from the unpledge list")
				up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
				continue
			}
			ss := strings.Split(refID, ",")
			if len(ss) != 3 {
				up.log.Error("Invalid reference ID, removing the token from the unpledge list")
				up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
				continue
			}
			tt, err := strconv.ParseInt(ss[1], 10, 32)
			if err != nil {
				up.log.Error("Invalid reference ID, removing the token from the unpledge list", "err", err)
				up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
				continue
			}
			blk, err = up.w.GetTokenBlock(ss[0], int(tt), ss[2])
			if err != nil {
				up.log.Error("Failed to get transaction block, removing the token from the unpledge list", "err", err)
				up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
				continue
			}
		}
		nb := block.InitBlock(blk, nil)
		if nb == nil {
			up.log.Error("Invalid transaction block, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		tid := nb.GetTid()
		rdid := nb.GetReceiverDID()

		hash := sha2Hash256(t + rdid + bid)
		fileName := up.dir + t + ".proof"
		f, err := os.Create(fileName)
		if err != nil {
			up.log.Error("Failed to create file, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}

		// ::TODO:: need to update
		dl := Difficultlevel
		targetHash := tid[len(tid)-dl:]
		f.WriteString(fmt.Sprintf("%d\n", dl))
		f.WriteString(hash + "\n")
		count := 1
		for {
			hash = sha3Hash256(hash)
			if targetHash == hash[len(hash)-dl:] {
				f.WriteString(hash + "\n")
				break
			}
			if count%RecordInterval == 0 {
				f.WriteString(hash + "\n")
			}
			count++
		}
		f.Close()
		if up.cb == nil {
			up.log.Error("Callback function not set, removing the token from the unpledge list")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		err = up.cb(t, fileName)
		if err != nil {
			up.log.Error("Error in unpledge alback, removing the token from the unpledge list", "err", err)
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		et := time.Now()
		df := et.Sub(st)
		up.log.Info("Unpledging completed for the token " + t + " in " + df.String())
		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
	}
}

// func (un *UnPledge) proofverification(tokenID string, proof []string) error {

// 	for {
// 		TokenID := tokenID
// 		ReceiverDID, err := un.GetLastBlockReceiverDID(TokenID)
// 		if err != nil {
// 			un.log.Error("Unable to fetch Receiver DID from last block")
// 			break
// 		}
// 		TransactionID, err := un.GetLastBlockTransactionID(TokenID)
// 		if err != nil {
// 			un.log.Error("Unable to fetch Transaction ID from last block")
// 		}

// 		ValueToBeHashed, err := un.Concat(TokenID, ReceiverDID)
// 		if err != nil {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		}

// 		valueHashed := un.CalcSHA_256Hash(ValueToBeHashed)
// 		if proof[0] == "" {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		}
// 		dl := un.Difficultlevel()
// 		if proof[0] != strconv.Itoa(dl) {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		}

// 		if proof[1] != valueHashed {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		}

// 		proofToVerify := proof[1:] // Exculding firstline (Difficuilty level)
// 		lenProof := len(proof)
// 		lenProoftoVerify := len(proofToVerify)
// 		l := lenProoftoVerify / 2

// 		firstHalf := proof[1 : l-1]
// 		secondHalf := proof[l+1 : lenProoftoVerify-1]

// 		rand.Seed(time.Now().UnixNano())

// 		randIndexInFH := rand.Intn(len(firstHalf) - 1)
// 		randIndexInSH := rand.Intn(len(secondHalf) - 1)

// 		RandomHashInFH := firstHalf[randIndexInFH]
// 		RandomHashInSH := secondHalf[randIndexInSH]

// 		TargetHashInFH := firstHalf[randIndexInFH+1]
// 		TargetHashInSH := secondHalf[randIndexInSH+1]

// 		if un.CalcSHA3_256Hash1000Times(RandomHashInFH) != TargetHashInFH || un.CalcSHA3_256Hash1000Times(RandomHashInSH) != TargetHashInSH {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		}
// 		var c int
// 		counter := 0
// 		target := proof[lenProof-3]

// 		SuffixLasthash := TransactionID[len(TransactionID)-dl:]

// 		for {
// 			targetHash := un.CalcSHA_3_256Hash(target)
// 			SuffixTarget := targetHash[len(targetHash)-dl:]

// 			if SuffixTarget == SuffixLasthash {
// 				c = counter
// 				break
// 			}
// 			counter++
// 			target = targetHash
// 		}
// 		if c > 1000 {
// 			un.log.Error("Unable to verify proof")
// 			break
// 		} else {
// 			un.log.Debug("Proof Verified")
// 		}
// 		break
// 	}
// 	return nil
// }
