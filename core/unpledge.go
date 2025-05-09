package core

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/rubixchain/rubixgoplatform/core/wallet"

	// "github.com/rubixchain/rubixgoplatform/core/model"
	tkn "github.com/rubixchain/rubixgoplatform/token"
)

const pledgePeriodInSeconds int = 7 * 24 * 60 * 60 // Pledging period: 7 days

func (c *Core) ForceUnpledgePOWBasedPledgedTokens() error {
	// Load data from UnpledgeQueueInfo table
	unpledgeQueueInfo, err := c.w.Migration_GetUnpledgeQueueInfo()
	if err != nil {
		return err
	}

	// unpledge all POW based pledged tokens
	for _, info := range unpledgeQueueInfo {
		pledgeToken := info.Token
		pledgeTokenType, err := getTokenType(c.w, pledgeToken, c.testNet)
		if err != nil {
			return fmt.Errorf("failed to unpledge POW based pledge token %v, err: %v", pledgeToken, err)
		}
		pledgeTokenOwner, err := getTokenOwner(c.w, pledgeToken)
		if err != nil {
			return fmt.Errorf("failed to unpledge POW based pledge token %v, err: %v", pledgeToken, err)
		}

		_, _, err = unpledgeToken(c, pledgeToken, pledgeTokenType, pledgeTokenOwner, "")
		if err != nil {
			c.log.Error("failed to unpledge POW based pledge token %v, err: %v", pledgeToken, err)
			return fmt.Errorf("failed to unpledge POW based pledge token %v, err: %v", pledgeToken, err)
		}
	}

	// Drop the UnpledgeSequence table
	tableDropErr := c.w.Migration_DropUnpledgeQueueTable()
	if tableDropErr != nil {
		return tableDropErr
	}

	return nil
}

func (c *Core) InititateUnpledgeProcess() (string, error) {
	c.log.Info("Unpledging process has started...")
	var totalUnpledgeAmount float64 = 0.0

	// Get the list of transactions from the unpledgeQueue table
	unpledgeSequenceInfo, err := c.w.GetUnpledgeSequenceDetails()
	if err != nil {
		return "", err
	}
	if len(unpledgeSequenceInfo) == 0 {
		return "No tokens present to unpledge", nil
	}

	var pledgeInformation []*wallet.PledgeInformation

	for _, info := range unpledgeSequenceInfo {
		var readyToUnpledge bool = false

		// Get all the token hashes by their transaction ID.
		// If there are no records found, that means all the tokens have changed their token, initiate unpledge
		// Else check if the pledging period has passed for all tokens, if yes, then unpledge. Else wait for the CheckPeriod
		tokenStateHashDetails, err := c.w.GetTokenStateHashByTransactionID(info.TransactionID)
		if err != nil {
			c.log.Error(fmt.Sprintf("error occured while fetching token state hashes for transaction ID: %v", info.TransactionID))
			return "", err
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
				return "", err
			}

			creditStorageErr := c.w.StoreCredit(info.TransactionID, info.QuorumDID, pledgeInformation)
			if creditStorageErr != nil {
				errMsg := fmt.Errorf("failed while storing credits, err: %v", creditStorageErr.Error())
				c.log.Error(errMsg.Error())
				return "", errMsg
			}

			removeUnpledgeSequenceInfoErr := c.w.RemoveUnpledgeSequenceInfo(info.TransactionID)
			if removeUnpledgeSequenceInfoErr != nil {
				errMsg := fmt.Errorf("failed to remove unpledgeSequenceInfo record for transaction: %v, error: %v", info.TransactionID, removeUnpledgeSequenceInfoErr)
				c.log.Error(errMsg.Error())

				// Remove the corresponding stored credit
				creditRemovalErr := c.w.RemoveCredit(info.TransactionID)
				if creditRemovalErr != nil {
					errMsg := fmt.Errorf("failed to remove credit for transaction ID: %v", creditRemovalErr)
					c.log.Error(errMsg.Error())
					return "", errMsg
				}

				return "", errMsg
			}
			c.UpdatePledgeStatus(strings.Split(info.PledgeTokens, ","), info.QuorumDID)
			unpledgeAmountForTransaction, err := c.getTotalAmountFromTokenHashes(strings.Split(info.PledgeTokens, ","))
			if err != nil {
				return "", fmt.Errorf("failed while getting total pledge amount for transaction id: %v, err: %v", info.TransactionID, err)
			}

			totalUnpledgeAmount += unpledgeAmountForTransaction
			c.log.Info(fmt.Sprintf("Unpledging for transaction %v was successful and credits have been awarded. Total Unpledge Amount: %v RBT", info.TransactionID, unpledgeAmountForTransaction))
		}
	}

	if totalUnpledgeAmount > 0 {
		return fmt.Sprintf("Unpledging of pledged tokens was successful, Total Unpledge Amount: %v RBT", totalUnpledgeAmount), nil
	} else {
		return "No tokens present to unpledge", nil
	}
}

func unpledgeToken(c *Core, pledgeToken string, pledgeTokenType int, quorumDID string, transactionId string) (pledgeID string, unpledgeId string, err error) {
	b := c.w.GetLatestTokenBlock(pledgeToken, pledgeTokenType)
	if b == nil {
		c.log.Error("Failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", pledgeTokenType)
		return "", "", fmt.Errorf("failed to unpledge invalid tokne chain block for token ", pledgeToken, " having token type as ", pledgeTokenType)
	}

	if b.GetTransType() != block.TokenPledgedType {
		c.log.Error(fmt.Sprintf("Unpledging the token :", pledgeToken))
		err := c.w.UpdateUnpledgedTokenStatus(quorumDID, pledgeToken, pledgeTokenType)
		if err != nil {
			c.log.Error("Failed to update un pledge token", "err", err)
			return "", "", err
		}
		if transactionId != "" {
			err := c.w.RemoveUnpledgeSequenceInfo(transactionId)
			if err != nil {
				c.log.Error("Failed to remove unpledge sequence info", "err", err)
				return "", "", err
			}
		}
	}

	pledgeID, err = b.GetBlockID(pledgeToken)
	if err != nil {
		errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
		c.log.Error(errMsg)
		return "", "", errors.New(errMsg)
	}

	ctcb := make(map[string]*block.Block)
	tsb := make([]block.TransTokens, 0)

	ts := block.TransTokens{
		Token:     pledgeToken,
		TokenType: pledgeTokenType,
	}

	dc, ok := c.qc[quorumDID]
	if !ok {
		c.log.Error("Failed to get quorum did crypto")
		return "", "", fmt.Errorf("failed to get quorum did crypto")
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
		return "", "", fmt.Errorf("failed to create new token chain block")
	}

	err = nb.UpdateSignature(dc)
	if err != nil {
		c.log.Error("Failed to update the signature", "err", err)
		return "", "", fmt.Errorf("failed to update the signature")
	}

	err = c.w.CreateTokenBlock(nb)
	if err != nil {
		c.log.Error("Failed to update token chain block", "err", err)
		return "", "", err
	}

	err = c.w.UnpledgeWholeToken(quorumDID, pledgeToken, pledgeTokenType)
	if err != nil {
		c.log.Error("Failed to update un pledge token", "err", err)
		return "", "", err
	}

	unpledgeId, err = nb.GetBlockID(pledgeToken)
	if err != nil {
		errMsg := fmt.Sprintf("failed while unpledging token %v, unable to fetch block ID", pledgeToken)
		c.log.Error(errMsg)
		return "", "", errors.New(errMsg)
	}

	return
}

func getTokenType(w *wallet.Wallet, tokenHash string, isTestnet bool) (int, error) {
	var tokenType int = -1

	walletToken, err := w.ReadToken(tokenHash)
	if err != nil {
		return tokenType, err
	}

	if isTestnet {
		if walletToken.TokenValue == 1 {
			tokenType = tkn.TestTokenType
		} else if walletToken.TokenValue < 1 {
			tokenType = tkn.TestPartTokenType
		}
	} else {
		if walletToken.TokenValue == 1 {
			tokenType = tkn.RBTTokenType
		} else if walletToken.TokenValue < 1 {
			tokenType = tkn.PartTokenType
		}
	}

	return tokenType, nil
}

func getTokenOwner(w *wallet.Wallet, tokenHash string) (string, error) {
	walletToken, err := w.ReadToken(tokenHash)
	if err != nil {
		return "", err
	}

	return walletToken.DID, nil
}

func unpledgeAllTokens(c *Core, transactionID string, pledgeTokens string, quorumDID string) ([]*wallet.PledgeInformation, error) {
	c.log.Debug(fmt.Sprintf("Executing Callback for tx for unpledging: %v", transactionID))

	var pledgeInfoList []*wallet.PledgeInformation = make([]*wallet.PledgeInformation, 0)
	pledgeTokensList := strings.Split(pledgeTokens, ",")

	if len(pledgeTokensList) == 0 {
		return nil, fmt.Errorf("expected atleast one pledged token for unpledging")
	}

	for _, pledgeToken := range pledgeTokensList {
		pledgeTokenType, err := getTokenType(c.w, pledgeToken, c.testNet)
		if err != nil {
			return nil, fmt.Errorf("failed while unpledging token %v, err: %v", pledgeToken, err)
		}

		pledgeTokenBlockID, unpledgeTokenBlockID, err := unpledgeToken(c, pledgeToken, pledgeTokenType, quorumDID, transactionID)
		if err != nil {
			return nil, err
		}

		// Add pledge and unpledge block information of a Pledged token
		pledgeInfoList = append(pledgeInfoList, &wallet.PledgeInformation{
			TokenID:         pledgeToken,
			TokenType:       pledgeTokenType,
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
