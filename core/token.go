package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/util"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

type TokenPublish struct {
	Token string `json:"token"`
}

func (c *Core) getTokens(did string, amount float64) ([]string, []string, bool) {
	return nil, nil, true
}

func (c *Core) removeTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: remove the tokens from the bank
	return nil
}

func (c *Core) releaseTokens(did string, wholeTokens []string, partTokens []string) error {
	// ::TODO:: releae the tokens which is lokced for the transaction
	return nil
}

func (c *Core) GetAccountInfo(did string) (*model.RBTInfo, error) {
	wt, err := c.w.GetAllWholeTokens(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		return nil, fmt.Errorf("Failed to get tokens")
	}
	pt, err := c.w.GetAllPartTokens(did)
	if err != nil {
		c.log.Error("Failed to get tokens", "err", err)
		return nil, fmt.Errorf("Failed to get tokens")
	}
	info := &model.RBTInfo{
		BasicResponse: model.BasicResponse{
			Status:  true,
			Message: "RBT account info",
		},
	}
	for _, t := range wt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.WholeRBT++
		case wallet.TokenIsLocked:
			info.LockedWholeRBT++
		case wallet.TokenIsPledged:
			info.PledgedWholeRBT++
		}
	}
	for _, t := range pt {
		switch t.TokenStatus {
		case wallet.TokenIsFree:
			info.PartRBT++
		case wallet.TokenIsLocked:
			info.LockedPartRBT++
		case wallet.TokenIsPledged:
			info.PledgedPartRBT++
		}
	}
	return info, nil
}

func (c *Core) GenerateTestTokens(reqID string, num int, did string) error {
	if !c.testNet {
		return fmt.Errorf("This operation only avialable in test net")
	}
	dc, err := c.SetupDID(reqID, did)
	if err != nil {
		return fmt.Errorf("DID is not exist")
	}

	for i := 0; i < num; i++ {
		m := make(map[string]string)
		m["timeStamp"] = time.Now().String()
		mb, err := json.Marshal(m)
		if err != nil {
			c.log.Error("Failed to do json marshal (timestamp)", "err", err)
			return fmt.Errorf("failed to do json marshal")
		}
		rb := util.GetRandBytes(16)

		rac := make(map[string]interface{})
		rac[wallet.RACTypeKey] = wallet.RACTestTokenType
		rac[wallet.RACVersionKey] = wallet.RACTestTokenVersion
		rac[wallet.RACDidKey] = did
		rac[wallet.RACTotalSupplyKey] = 1
		rac[wallet.RACTokenCountKey] = 1
		rac[wallet.RACCreatorInputKey] = string(mb)
		rac[wallet.RACNonceKey] = base64.RawURLEncoding.EncodeToString(rb)

		ha, err := wallet.RAC2Hash(rac)
		if err != nil {
			c.log.Error("Failed to calculate rac hash", "err", err)
			return err
		}
		sig, err := dc.PvtSign(ha)
		if err != nil {
			c.log.Error("Failed to get rac signature", "err", err)
			return err
		}
		rac[wallet.RACSignKey] = base64.RawURLEncoding.EncodeToString(sig)

		tb, err := json.Marshal(rac)
		if err != nil {
			c.log.Error("Failed to convert rac to json string", "err", err)
			return err
		}
		nb := bytes.NewBuffer(tb)
		id, err := c.ipfs.Add(nb)
		if err != nil {
			c.log.Error("Failed to add token to network", "err", err)
			return err
		}

		tcb := wallet.TokenChainBlock{
			TransactionType: wallet.TokenGeneratedType,
			TokenOwner:      did,
			Comment:         "Token generated at " + time.Now().String(),
		}

		ctcb := make(map[string]interface{})
		ctcb[id] = nil

		ntcb := wallet.CreateTCBlock(ctcb, &tcb)

		if ntcb == nil {
			c.log.Error("Failed to create new token chain block")
			return fmt.Errorf("Failed to create new token chain block")
		}

		hash, ok := ntcb[wallet.TCBlockHashKey]
		if !ok {
			c.log.Error("Invalid new token chain block, missing block hash")
			return fmt.Errorf("Invalid new token chain block, missing block hash")
		}

		bid, err := wallet.GetBlockID(id, ntcb)

		if err != nil {
			c.log.Error("Failed to get block id", "err", err)
			return fmt.Errorf("Failed to get block id")
		}

		sig, err = dc.PvtSign([]byte(hash.(string)))
		if err != nil {
			c.log.Error("Failed to get did signature", "err", err)
			return fmt.Errorf("Failed to get did signature")
		}

		ntcb[wallet.TCSignatureKey] = util.HexToStr(sig)

		t := &wallet.Token{
			TokenID:      id,
			TokenDetials: string(tb),
			DID:          did,
			TokenChainID: bid,
			TokenStatus:  wallet.TokenIsFree,
		}
		err = c.w.AddLatestTokenBlock(id, ntcb)
		if err != nil {
			c.log.Error("Failed to add token chain", "err", err)
			return err
		}
		err = c.w.CreateToken(t)
		if err != nil {
			c.log.Error("Failed to create token", "err", err)
			return err
		}
	}
	return nil
}

func (c *Core) tokenStatusCallback(peerID string, topic string, data []byte) {
	// c.log.Debug("Recevied token status request")
	// var tp TokenPublish
	// err := json.Unmarshal(data, &tp)
	// if err != nil {
	// 	return
	// }
	// c.log.Debug("Token recevied", "token", tp.Token)
}
