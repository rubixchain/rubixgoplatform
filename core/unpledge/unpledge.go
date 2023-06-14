package unpledge

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/logger"
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

func (up *UnPledge) RunUnpledge8HourlyThread() error {
	go up.RunUnpledge8Hourly()
	return nil
}
func (up *UnPledge) RunUnpledge8Hourly() {
	for {
		go up.runUnpledge()

		// Sleep for 8 hours
		duration := time.Duration(90) * time.Second
		time.Sleep(duration)
	}
}

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

func (up *UnPledge) RunUnpledge() error {
	go up.runUnpledge()
	return nil
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
		//st := time.Now()
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

		epochTimeString, err := b.GetBlockEpoch()
		if err != nil {
			up.log.Error("Failed to get the epoch time, removing the token from the unpledge list", err)
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}

		// Convert the epoch time string to an integer
		epochTime, err := strconv.ParseInt(epochTimeString, 10, 64)
		if err != nil {
			fmt.Println("Error parsing epoch time:", err)
			return
		}
		// Convert the epoch time to a time.Time value
		storedTime := time.Unix(epochTime, 0)
		// Calculate the duration between the stored time and current time
		duration := time.Since(storedTime)
		// Define a duration representing 24 hours
		twentyFourHours := 24 * time.Second
		// Compare the duration with 24 hours
		if duration >= twentyFourHours {
			fmt.Println("24 hours have elapsed.")
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		} else {
			fmt.Println("Less than 24 hours have elapsed.")
		}

		//bid, err := b.GetBlockID(t)
		//if err != nil {
		//	up.log.Error("Failed to get the block id, removing the token from the unpledge list")
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//if b.GetTransType() != block.TokenPledgedType {
		//	up.log.Error("Token is not in pledged state, removing the token from the unpledge list")
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//blk := b.GetTransBlock()
		//if blk == nil {
		//	refID := b.GetRefID()
		//	if refID == "" {
		//		up.log.Error("Token block missing transaction block, removing the token from the unpledge list")
		//		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//		continue
		//	}
		//	ss := strings.Split(refID, ",")
		//	if len(ss) != 3 {
		//		up.log.Error("Invalid reference ID, removing the token from the unpledge list")
		//		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//		continue
		//	}
		//	tt, err := strconv.ParseInt(ss[1], 10, 32)
		//	if err != nil {
		//		up.log.Error("Invalid reference ID, removing the token from the unpledge list", "err", err)
		//		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//		continue
		//	}
		//	blk, err = up.w.GetTokenBlock(ss[0], int(tt), ss[2])
		//	if err != nil {
		//		up.log.Error("Failed to get transaction block, removing the token from the unpledge list", "err", err)
		//		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//		continue
		//	}
		//}
		//nb := block.InitBlock(blk, nil)
		//if nb == nil {
		//	up.log.Error("Invalid transaction block, removing the token from the unpledge list")
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//tid := nb.GetTid()
		//rdid := nb.GetReceiverDID()
		//
		//hash := sha2Hash256(t + rdid + bid)
		//fileName := up.dir + t + ".proof"
		//f, err := os.Create(fileName)
		//if err != nil {
		//	up.log.Error("Failed to create file, removing the token from the unpledge list")
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//
		//dl := Difficultlevel
		//targetHash := tid[len(tid)-dl:]
		//f.WriteString(fmt.Sprintf("%d\n", dl))
		//f.WriteString(hash + "\n")
		//count := 1
		//for {
		//	hash = sha3Hash256(hash)
		//	if targetHash == hash[len(hash)-dl:] {
		//		f.WriteString(hash + "\n")
		//		break
		//	}
		//	if count%RecordInterval == 0 {
		//		f.WriteString(hash + "\n")
		//	}
		//	count++
		//}
		//f.Close()
		//if up.cb == nil {
		//	up.log.Error("Callback function not set, removing the token from the unpledge list")
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//err = up.cb(t, fileName)
		//if err != nil {
		//	up.log.Error("Error in unpledge alback, removing the token from the unpledge list", "err", err)
		//	up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
		//	continue
		//}
		//et := time.Now()
		//df := et.Sub(st)
		up.log.Info("Unpledging completed for the token " + t)
		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
	}
}

func (up *UnPledge) ProofVerification(tokenID string) (bool, error) {
	tt := token.RBTTokenType
	blk := up.w.GetLatestTokenBlock(tokenID, tt)

	epochTimeString, err := blk.GetBlockEpoch()
	if err != nil {
		up.log.Error("Failed to get the epoch time, removing the token from the unpledge list")
		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", tt)
		return false, err
	}

	// Convert the epoch time string to an integer
	epochTime, err := strconv.ParseInt(epochTimeString, 10, 64)
	if err != nil {
		fmt.Println("Error parsing epoch time:", err)
		return false, err
	}
	// Convert the epoch time to a time.Time value
	storedTime := time.Unix(epochTime, 0)
	// Calculate the duration between the stored time and current time
	duration := time.Since(storedTime)
	// Define a duration representing 24 hours
	twentyFourHours := 24 * time.Hour
	// Compare the duration with 24 hours
	if duration >= twentyFourHours {
		fmt.Println("24 hours have elapsed.")
		return true, nil
	} else {
		fmt.Println("Less than 24 hours have elapsed.")
		return false, err
	}

	//valueHashed := sha2Hash256(tokenID + rdid + bid)
	//if proof[0] == "" {
	//	err := errors.New("First line of proof empty. Unable to verify proof file")
	//	up.log.Error(err.Error())
	//	return false, err
	//}
	//dl := Difficultlevel
	//if proof[0] != strconv.Itoa(dl) {
	//	err := errors.New("First line of proof mismatch. Unable to verify proof file")
	//	up.log.Error(err.Error())
	//	return false, err
	//}
	//
	//if proof[1] != valueHashed {
	//	err := errors.New("Second line of proof mismatch. Unable to verify proof file")
	//	up.log.Error(err.Error())
	//	return false, err
	//}
	//
	//proofToVerify := proof[1:] // Exculding firstline (Difficuilty level)
	//lenProof := len(proof)
	//lenProoftoVerify := len(proofToVerify)
	//l := lenProoftoVerify / 2
	//
	//firstHalf := proof[1 : l-1]
	//secondHalf := proof[l : lenProoftoVerify-2]
	//
	//rand.Seed(time.Now().UnixNano())
	//
	//randIndexInFH := rand.Intn(len(firstHalf) - 2)
	//randIndexInSH := rand.Intn(len(secondHalf) - 2)
	//
	//randomHashInFH := firstHalf[randIndexInFH]
	//randomHashInSH := secondHalf[randIndexInSH]
	//
	//targetHashInFH := firstHalf[randIndexInFH+1]
	//targetHashInSH := secondHalf[randIndexInSH+1]
	//
	//if sha3Hash256Loop(randomHashInFH) != targetHashInFH || sha3Hash256Loop(randomHashInSH) != targetHashInSH {
	//	err := errors.New("Random hash verification fail. Unable to verify proof file")
	//	up.log.Error(err.Error())
	//	return false, err
	//}
	//
	//var c int
	//counter := 0
	//target := proof[lenProof-2]
	//lastHash := proof[lenProof-1]
	//suffixLasthash := lastHash[len(lastHash)-dl:]
	//
	//for {
	//	targetHash := sha3Hash256(target)
	//	suffixTarget := targetHash[len(targetHash)-dl:]
	//
	//	if suffixTarget == suffixLasthash || counter > RecordInterval {
	//		c = counter
	//		break
	//	}
	//	counter++
	//	target = targetHash
	//}
	//if c > RecordInterval-1 || suffixLasthash != tid[len(tid)-dl:] {
	//	up.log.Error("Last line of proof mismatch, Unable to verify proof file")
	//	return false, err
	//} else {
	//	up.log.Info("Proof Verified for " + tokenID)
	//	return true, nil
	//}
}
