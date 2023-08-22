package unpledge

import (
	"github.com/rubixchain/rubixgoplatform/block"
	"sync"
	"time"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/token"
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
		duration := time.Duration(8) * time.Hour
		time.Sleep(duration)
	}
}

func (up *UnPledge) GetSelfTransferTokens(did string) []string {
	wt, err := up.w.GetAllWholeTokens(did)
	if err != nil {
		up.log.Error("Failed to get tokens", "err", err)
		//return model.DIDAccountInfo{}, fmt.Errorf("failed to get tokens")
	}
	var selectedTokens []string
	for i := range wt {
		tokenType := token.RBTTokenType
		if up.testNet {
			tokenType = token.TestTokenType
		}
		if wt[i].TokenStatus == wallet.TokenIsFree {
			b := up.w.GetLatestTokenBlock(wt[i].TokenID, tokenType)
			if b == nil {
				up.log.Error("Invalid token chain block")
			}
			if (b.GetTransType()) == block.TokenTransferredType {
				up.log.Debug("its a transferred token")
				timeString, err := b.GetBlockEpoch()
				up.log.Debug("Epoch Time Stored: " + timeString)
				if err != nil {
					up.log.Error("Failed to get the epoch time, removing the token from the unpledge list", err)
				}
				layout := "2006-01-02 15:04:05.999999 -0700 MST "

				timeString = timeString[0:36]
				storedTime, err := time.Parse(layout, timeString)
				if err != nil {
					up.log.Error("Error:", err)
				}

				elapsed := time.Since(storedTime)
				if elapsed >= 384*time.Hour {
					up.log.Info("16 days have elapsed.")
					selectedTokens = append(selectedTokens, wt[i].TokenID)
				} else {
					up.log.Info("Less than 16 days have elapsed.")
				}
			} else {
				up.log.Debug("its not a transferred token")
			}
		}
	}
	return selectedTokens
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

//func sha2Hash256(input string) string {
//	hash := sha256.Sum256([]byte(input))
//	hashedInput := fmt.Sprintf("%x", hash)
//	return hashedInput
//}
//
//func sha3Hash256(input string) string {
//	hash := sha3.Sum256([]byte(input))
//	hashedInput := fmt.Sprintf("%x", hash)
//	return hashedInput
//}
//
//func sha3Hash256Loop(input string) string {
//	var hashedInput string
//	for i := 0; i < RecordInterval; i++ {
//		hash := sha3.Sum256([]byte(input))
//		hashedInput = fmt.Sprintf("%x", hash)
//		input = hashedInput
//	}
//	return hashedInput
//}

//func (up *UnPledge) isRunning() bool {
//	up.l.Lock()
//	s := up.running
//	up.l.Unlock()
//	return s
//}

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
	pledgedTokens, err := up.w.GetAllPledgedTokens()

	if err != nil {
		up.log.Debug("Pledged list is empty")
	}
	for i := range pledgedTokens {
		t := pledgedTokens[i].TokenID
		//var list UnpledgeTokenList
		//err := up.s.Read(UnpledgeQueueTable, &list, "token != ?", wallet.TokenIsPledged)
		//if err != nil {
		//	up.log.Info("All tokens are unplegded")
		//	break
		//}
		//
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

		//Epoch time comparison for Unpledging
		timeString, err := b.GetBlockEpoch()
		up.log.Debug("Epoch Time Stored: " + timeString)
		if err != nil {
			up.log.Error("Failed to get the epoch time, removing the token from the unpledge list", err)
			up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
			continue
		}
		layout := "2006-01-02 15:04:05.999999 -0700 MST "

		timeString = timeString[0:36]
		up.log.Debug("substring: ", timeString)

		storedTime, err := time.Parse(layout, timeString)
		if err != nil {
			up.log.Error("Error:", err)
			return
		}

		elapsed := time.Since(storedTime)
		if elapsed >= 384*time.Hour {
			up.log.Info("16 days have elapsed.")
			did := b.GetOwner()
			up.w.UnpledgeWholeToken(did, t, tt)

		} else {
			up.log.Info("Less than 16 days have elapsed.")
		}

		up.log.Info("Unpledging completed for the token " + t)
		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", t)
	}

}

//func (up *UnPledge) ProofVerification(tokenID string) (bool, error) {
//	tt := token.RBTTokenType
//	blk := up.w.GetLatestTokenBlock(tokenID, tt)
//
//	epochTimeString, err := blk.GetBlockEpoch()
//	if err != nil {
//		up.log.Error("Failed to get the epoch time, removing the token from the unpledge list")
//		up.s.Delete(UnpledgeQueueTable, &UnpledgeTokenList{}, "token=?", tt)
//		return false, err
//	}
//
//	// Convert the epoch time string to an integer
//	epochTime, err := strconv.ParseInt(epochTimeString, 10, 64)
//	if err != nil {
//		fmt.Println("Error parsing epoch time:", err)
//		return false, err
//	}
//	// Convert the epoch time to a time.Time value
//	storedTime := time.Unix(epochTime, 0)
//	// Calculate the duration between the stored time and current time
//	duration := time.Since(storedTime)
//	// Define a duration representing 24 hours
//	twentyFourHours := 24 * time.Hour
//	// Compare the duration with 24 hours
//	if duration >= twentyFourHours {
//		fmt.Println("24 hours have elapsed.")
//		return true, nil
//	} else {
//		fmt.Println("Less than 24 hours have elapsed.")
//		return false, err
//	}
//
//}
