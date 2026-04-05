package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Quorums listening on "rubix_txns" event will check the `Tokens` attribute
// of incoming `TransactionInfo` (recieved via main Callback function of "rubix_txns")
// and will go through all Tokens. Quorum check if the TokenStateHash is present
// in their DB or not (for which they have pledged). If they have, then they can
// remove all the records from the TokenStateHash table
func (c *Core) CallBackQuorumUnpledge(tx *models.Transactions, did string) error {
	var txInfo models.TransactionInfo
	if err := json.Unmarshal(tx.Info, &txInfo); err != nil {
		return fmt.Errorf("failed to unmarshal transaction info, err: %v", err)
	}

	// Loop through all the tokens and gather previous transactionID and
	// pledge Token ID map
	var prevTransactionsSet map[string]struct{} = make(map[string]struct{})

	addToSet := func(id string) {
		if id != "" {
			prevTransactionsSet[id] = struct{}{}
		}
	}

	for _, rbtToken := range txInfo.Tokens.RBT {
		addToSet(rbtToken.PreviousTransactionID)
	}

	for _, ftToken := range txInfo.Tokens.FT {
		addToSet(ftToken.PreviousTransactionID)
	}

	for _, nftToken := range txInfo.Tokens.NFT {
		addToSet(nftToken.PreviousTransactionID)
	}

	for _, smartContractToken := range txInfo.Tokens.SmartContract {
		addToSet(smartContractToken.PreviousTransactionID)
	}

	if len(prevTransactionsSet) == 0 {
		return nil
	}

	var prevTransactionList []string = make([]string, 0, len(prevTransactionsSet))
	for prevTransaction := range prevTransactionsSet {
		prevTransactionList = append(prevTransactionList, prevTransaction)
	}

	transactionToUnpledge, err := c.w.CheckTxnsPresentInUnpledgeSequenceInfo(prevTransactionList)
	if err != nil {
		return fmt.Errorf("CallBackQuorumUnpledge: failed to get transactions from `unpledge_sequence_info` table, err: %v", err)
	}

	for _, txToUnpledge := range transactionToUnpledge {
		quorumDC, ok := c.qc[did]
		if !ok {
			return fmt.Errorf("CallBackQuorumUnpledge: quorum DID not setup: %s", did)
		}
		if err := c.w.UnpledgeV2(context.Background(), txToUnpledge, tx.ID, did, quorumDC); err != nil {
			c.log.Error("CallBackQuorumUnpledge: UnpledgeV2 failed",
				"prevTxID", txToUnpledge,
				"did", did,
				"err", err,
			)
		}
	}

	return nil
}
