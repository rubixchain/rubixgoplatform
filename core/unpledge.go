package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"

	// "github.com/rubixchain/rubixgoplatform/core/model"
	tkn "github.com/rubixchain/rubixgoplatform/token"
)

const UnpledgeQueueTable string = "unpledgequeue"
const pledgePeriodInSeconds int = 100 //7 * 24 * 60 * 60

type TransTokenBlock struct {
	TokenID       string `json:"token_id"`
	TransferBlock string `json:"token_block"`
}

type PledgeUnpledgeBlock struct {
	TokenID       string `json:"token_id"`
	PledgeBlock   string `json:"pledge_block"`
	UnpledgeBlock string `json:"unpledge_block"`
}

type PledgeInformation struct {
	TokenID         string `json:"token_id"`
	TokenType       int    `json:"token_type"`
	PledgeBlockID   string `json:"pledge_block_id"`
	UnpledgeBlockID string `json:"unpledge_block_id"`
	QuorumDID       string `json:"quorum_did"`
	TransactionID   string `json:"transaction_id"`
}

type UnpledgeQueueInfo struct {
	TransactionID string `gorm:"column:tx_id;primaryKey"`
	PledgeTokens  string `gorm:"column:pledge_tokens"`
	Epoch         int64  `gorm:"column:epoch"`
	QuorumDID     string `gorm:"column:quorum_did"`
}

func (c *Core) InititateUnpledgeProcess() error {
	// Get the list of transactions from the unpledgeQueue table
	var UnpledgeQueueInfoList []UnpledgeQueueInfo
	err := c.s.Read(UnpledgeQueueTable, &UnpledgeQueueInfoList, "tx_id != ?", "")
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			c.log.Info("No tokens left to unpledge")
		} else {
			return err
		}
	}

	var pledgeInformation []*PledgeInformation

	for _, info := range UnpledgeQueueInfoList {
		var readyToUnpledge bool = false

		// Get all the token hashes by their transaction ID.
		// If there are no records found, that means all the tokens have changed their token, initiate unpledge
		// Else check if the pledging period has passed for all tokens, if yes, then unpledge. Else wait for the CheckPeriod
		tokenStateHashDetails, err := c.w.GetTokenStateHashByTransactionID(info.TransactionID)
		if err != nil {
			c.log.Error(fmt.Sprintf("error occured while fetching token state hashes for transaction ID: %v", info.TransactionID))
			return err
		}

		// Not all tokens have undergone state change
		if len(tokenStateHashDetails) > 0 {
			// check if pledging period has passed or not
			currentTimeEpoch := time.Now().Unix()
			transactionEpoch := info.Epoch

			if (currentTimeEpoch - transactionEpoch) > int64(pledgePeriodInSeconds) {
				readyToUnpledge = true
				c.log.Debug("Tokens have gone past their pledge duration. Proceeding to unpledge...")
			}
		} else {
			readyToUnpledge = true
			c.log.Debug("All tokens have undergone state chanage. Proceeding to unpledge...")

		}

		if readyToUnpledge {
			pledgeInformation, err = unpledgeAllTokens(c, info.TransactionID, info.PledgeTokens, info.QuorumDID)
			if err != nil {
				c.log.Error(fmt.Sprintf("failed while unpledging tokens for transaction: %v", info.TransactionID))
				return err
			}

			creditStorageErr := storeCredit(c, info.TransactionID, info.QuorumDID, pledgeInformation)
			if creditStorageErr != nil {
				c.log.Error(fmt.Sprintf("failed while storing credits, err: %v", creditStorageErr.Error()))
				return err
			}

			c.s.Delete(UnpledgeQueueTable, &UnpledgeQueueInfo{}, "tx_id = ?", info.TransactionID)
			c.log.Info(fmt.Sprintf("Unpledging for tx %v are successful. Credits have been awarded", info.TransactionID))
		}
	}

	return nil
}

func storeCredit(c *Core, txID string, quorumDID string, pledgeInfo []*PledgeInformation) error {
	pledgeInfoBytes, err := json.Marshal(pledgeInfo)
	if err != nil {
		return fmt.Errorf("failed while marshalling credits: %v", err.Error())
	}
	pledgeInfoEncoded := base64.StdEncoding.EncodeToString(pledgeInfoBytes)

	credit := &wallet.Credit{
		DID:    quorumDID,
		Credit: pledgeInfoEncoded,
		Tx:     txID,
	}

	return c.s.Write(wallet.CreditStorage, credit)
}

func unpledgeAllTokens(c *Core, transactionID string, pledgeTokens string, quorumDID string) ([]*PledgeInformation, error) {
	c.log.Debug(fmt.Sprintf("Executing Callback for tx for unpledging: %v", transactionID))

	var pledgeInfoList []*PledgeInformation = make([]*PledgeInformation, 0)
	pledgeTokensList := strings.Split(pledgeTokens, ",")

	if len(pledgeTokensList) == 0 {
		return nil, fmt.Errorf("expected atleast one pledged token for unpledging")
	}

	for _, pledgeToken := range pledgeTokensList {
		var tokenValue float64
		var tokenType int

		// Read Token from token hash
		walletToken, err := c.w.ReadToken(pledgeToken)
		if err != nil {
			return nil, err
		}
		tokenValue = walletToken.TokenValue

		c.log.Debug(fmt.Sprintf("Tx: %v, Status of pledge token %v is %v", transactionID, pledgeToken, walletToken.TokenStatus))

		if c.testNet {
			if tokenValue == 1 {
				tokenType = tkn.TestTokenType
			} else if tokenValue < 1 {
				tokenType = tkn.TestPartTokenType
			}
		} else {
			if tokenValue == 1 {
				tokenType = tkn.RBTTokenType
			} else if tokenValue < 1 {
				tokenType = tkn.PartTokenType
			}
		}

		b := c.w.GetLatestTokenBlock(pledgeToken, tokenType)
		if b == nil {
			c.log.Error("Failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", tokenType)
			return nil, fmt.Errorf("Failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", tokenType)
		}

		if b.GetTransType() != block.TokenPledgedType {
			c.log.Error(fmt.Sprintf("failed while unpledging token %v, token must be in pledged state before unpledging", pledgeToken))
			return nil, fmt.Errorf("failed while unpledging token %v, token must be in pledged state before unpledging", pledgeToken)
		}

		pledgeTokenBlockID, err := b.GetBlockID(pledgeToken)
		if err != nil {
			errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
			c.log.Error(errMsg)
			return nil, errors.New(errMsg)
		}

		ctcb := make(map[string]*block.Block)
		tsb := make([]block.TransTokens, 0)

		ts := block.TransTokens{
			Token:     pledgeToken,
			TokenType: tokenType,
		}

		dc, ok := c.qc[quorumDID]
		if !ok {
			c.log.Error("Failed to get quorum did crypto")
			return nil, fmt.Errorf("failed to get quorum did crypto")
		}
		tsb = append(tsb, ts)
		ctcb[pledgeToken] = b
		currentTime := time.Now()

		tcb := block.TokenChainBlock{
			TransactionType: block.TokenUnpledgedType,
			TokenOwner:      quorumDID,
			TransInfo: &block.TransInfo{
				Comment: "Token is un pledged at " + currentTime.String(),
				Tokens:  tsb,
			},
			Epoch: int(currentTime.Unix()),
		}

		nb := block.CreateNewBlock(ctcb, &tcb)
		if nb == nil {
			c.log.Error("Failed to create new token chain block")
			return nil, fmt.Errorf("failed to create new token chain block")
		}

		err = nb.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update the signature", "err", err)
			return nil, fmt.Errorf("failed to update the signature")
		}

		err = c.w.CreateTokenBlock(nb)
		if err != nil {
			c.log.Error("Failed to update token chain block", "err", err)
			return nil, err
		}

		err = c.w.UnpledgeWholeToken(quorumDID, pledgeToken, tokenType)
		if err != nil {
			c.log.Error("Failed to update un pledge token", "err", err)
			return nil, err
		}

		unpledgeTokenBlockID, err := nb.GetBlockID(pledgeToken)
		if err != nil {
			errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
			c.log.Error(errMsg)
			return nil, errors.New(errMsg)
		}

		// Add pledge and unpledge block information of a Pledged token
		pledgeInfoList = append(pledgeInfoList, &PledgeInformation{
			TokenID:         pledgeToken,
			TokenType:       tokenType,
			PledgeBlockID:   pledgeTokenBlockID,
			UnpledgeBlockID: unpledgeTokenBlockID,
			QuorumDID:       quorumDID,
			TransactionID:   transactionID,
		})
	}

	// If the unpledging is happening after the pledging period, we can safely remove
	// the TokenStateHash table records for the input transactionID
	err := c.w.RemoveTokenStateHashByTransactionID(transactionID)
	if err != nil {
		return nil, err
	}

	return pledgeInfoList, nil
}

// type UnpledgeCBType func(transactionID string, pledgeTokens string, quorumDID string) ([]*PledgeInformation, error)
func (c *Core) ExecuteUnpledge(transactionID string, pledgeTokens string, quorumDID string) ([]*PledgeInformation, error) {
	c.log.Debug(fmt.Sprintf("Executing Callback for tx for unpledging: %v", transactionID))

	var pledgeInfoList []*PledgeInformation = make([]*PledgeInformation, 0)
	pledgeTokensList := strings.Split(pledgeTokens, ",")

	if len(pledgeTokensList) == 0 {
		return nil, fmt.Errorf("expected atleast one pledged token for unpledging")
	}

	for _, pledgeToken := range pledgeTokensList {
		var tokenValue float64
		var tokenType int

		// Read Token from token hash
		walletToken, err := c.w.ReadToken(pledgeToken)
		if err != nil {
			return nil, err
		}
		tokenValue = walletToken.TokenValue

		c.log.Debug(fmt.Sprintf("Tx: %v, Status of pledge token %v is %v", transactionID, pledgeToken, walletToken.TokenStatus))

		if c.testNet {
			if tokenValue == 1 {
				tokenType = tkn.TestTokenType
			} else if tokenValue < 1 {
				tokenType = tkn.TestPartTokenType
			}
		} else {
			if tokenValue == 1 {
				tokenType = tkn.RBTTokenType
			} else if tokenValue < 1 {
				tokenType = tkn.PartTokenType
			}
		}

		b := c.w.GetLatestTokenBlock(pledgeToken, tokenType)
		if b == nil {
			c.log.Error("Failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", tokenType)
			return nil, fmt.Errorf("Failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", tokenType)
		}

		if b.GetTransType() != block.TokenPledgedType {
			c.log.Error(fmt.Sprintf("failed while unpledging token %v, token must be in pledged state before unpledging", pledgeToken))
			return nil, fmt.Errorf("failed while unpledging token %v, token must be in pledged state before unpledging", pledgeToken)
		}

		pledgeTokenBlockID, err := b.GetBlockID(pledgeToken)
		if err != nil {
			errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
			c.log.Error(errMsg)
			return nil, errors.New(errMsg)
		}

		ctcb := make(map[string]*block.Block)
		tsb := make([]block.TransTokens, 0)

		ts := block.TransTokens{
			Token:     pledgeToken,
			TokenType: tokenType,
		}

		dc, ok := c.qc[quorumDID]
		if !ok {
			c.log.Error("Failed to get quorum did crypto")
			return nil, fmt.Errorf("failed to get quorum did crypto")
		}
		tsb = append(tsb, ts)
		ctcb[pledgeToken] = b
		currentTime := time.Now()

		tcb := block.TokenChainBlock{
			TransactionType: block.TokenUnpledgedType,
			TokenOwner:      quorumDID,
			TransInfo: &block.TransInfo{
				Comment: "Token is un pledged at " + currentTime.String(),
				Tokens:  tsb,
			},
			Epoch: int(currentTime.Unix()),
		}

		nb := block.CreateNewBlock(ctcb, &tcb)
		if nb == nil {
			c.log.Error("Failed to create new token chain block")
			return nil, fmt.Errorf("failed to create new token chain block")
		}

		err = nb.UpdateSignature(dc)
		if err != nil {
			c.log.Error("Failed to update the signature", "err", err)
			return nil, fmt.Errorf("failed to update the signature")
		}

		err = c.w.CreateTokenBlock(nb)
		if err != nil {
			c.log.Error("Failed to update token chain block", "err", err)
			return nil, err
		}

		err = c.w.UnpledgeWholeToken(quorumDID, pledgeToken, tokenType)
		if err != nil {
			c.log.Error("Failed to update un pledge token", "err", err)
			return nil, err
		}

		unpledgeTokenBlockID, err := nb.GetBlockID(pledgeToken)
		if err != nil {
			errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
			c.log.Error(errMsg)
			return nil, errors.New(errMsg)
		}

		// Add pledge and unpledge block information of a Pledged token
		pledgeInfoList = append(pledgeInfoList, &PledgeInformation{
			TokenID:         pledgeToken,
			TokenType:       tokenType,
			PledgeBlockID:   pledgeTokenBlockID,
			UnpledgeBlockID: unpledgeTokenBlockID,
			QuorumDID:       quorumDID,
			TransactionID:   transactionID,
		})
	}

	// If the unpledging is happening after the pledging period, we can safely remove
	// the TokenStateHash table records for the input transactionID
	err := c.w.RemoveTokenStateHashByTransactionID(transactionID)
	if err != nil {
		return nil, err
	}

	return pledgeInfoList, nil
}
