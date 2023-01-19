package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

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

		tc := make(map[string]interface{})

		rb = util.GetRandBytes(16)

		tc[wallet.TCTransTypeKey] = wallet.TokenGeneratedType
		tc[wallet.TCOwnerKey] = did
		tc[wallet.TCCommentKey] = "Token generated at " + time.Now().String()
		tc[wallet.TCTokenIDKey] = id
		tc[wallet.TCNonceKey] = base64.RawURLEncoding.EncodeToString(rb)

		hash, err := wallet.TC2HashString(tc)

		if err != nil {
			c.log.Error("Failed to calculate token chain hash", "err", err)
			return err
		}
		tc[wallet.TCBlockHashKey] = hash

		_, sig, err = dc.Sign(hash)
		if err != nil {
			c.log.Error("Failed to get did signature", "err", err)
			return fmt.Errorf("Failed to get did signature")
		}

		tc[wallet.TCSignatureKey] = util.HexToStr(sig)

		t := &wallet.Token{
			TokenID:      id,
			TokenDetials: string(tb),
			DID:          did,
			TokenChainID: hash,
			TokenStatus:  wallet.TokenIsFree,
		}
		err = c.w.AddLatestTokenBlock(id, tc)
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

func (c *Core) tokenStatusCallback(peerID string, data []byte) {
	// c.log.Debug("Recevied token status request")
	// var tp TokenPublish
	// err := json.Unmarshal(data, &tp)
	// if err != nil {
	// 	return
	// }
	// c.log.Debug("Token recevied", "token", tp.Token)
}
