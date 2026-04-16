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
	var transactionTokens []string = make([]string, 0)

	addToSet := func(id string) {
		if id != "" {
			prevTransactionsSet[id] = struct{}{}
		}
	}

	for _, rbtToken := range txInfo.Tokens.RBT {
		addToSet(rbtToken.PreviousTransactionID)
		transactionTokens = append(transactionTokens, rbtToken.TokenID)
	}

	for _, ftToken := range txInfo.Tokens.FT {
		addToSet(ftToken.PreviousTransactionID)
		transactionTokens = append(transactionTokens, ftToken.TokenID)
	}

	for _, nftToken := range txInfo.Tokens.NFT {
		addToSet(nftToken.PreviousTransactionID)
		transactionTokens = append(transactionTokens, nftToken.TokenID)
	}

	for _, smartContractToken := range txInfo.Tokens.SmartContract {
		addToSet(smartContractToken.PreviousTransactionID)
		transactionTokens = append(transactionTokens, smartContractToken.TokenID)
	}

	if len(prevTransactionsSet) == 0 {
		return nil
	}

	var prevTransactionList []string = make([]string, 0, len(prevTransactionsSet))
	for prevTransaction := range prevTransactionsSet {
		prevTransactionList = append(prevTransactionList, prevTransaction)
	}

	transactionToUnpledge, err := c.w.CheckTxnsPresentInUnpledgeSequenceInfo(prevTransactionList, did, transactionTokens)
	if err != nil {
		return fmt.Errorf("CallBackQuorumUnpledge: failed to get transactions from `unpledge_sequence_info` table for did %q, err: %v", did, err)
	}

	// Emit a debug log for prevTxIDs that are NOT present in unpledge_sequence_info
	// for this quorum — this quorum never pledged for those txs, so there is nothing
	// to unpledge. Logging here preserves observability for the no-match case.
	matchedSet := make(map[string]struct{}, len(transactionToUnpledge))
	for _, txID := range transactionToUnpledge {
		matchedSet[txID] = struct{}{}
	}
	for _, prevTxID := range prevTransactionList {
		if _, ok := matchedSet[prevTxID]; !ok {
			c.log.Debug("CallBackQuorumUnpledge: prevTxID not in unpledge_sequence_info — skip (not pledged by this quorum)",
				"prevTxID", prevTxID,
				"did", did,
			)
		}
	}

	for _, txToUnpledge := range transactionToUnpledge {
		_, ok := c.qc[did]
		if !ok {
			return fmt.Errorf("CallBackQuorumUnpledge: quorum DID not setup: %s", did)
		}
		if err := c.UnpledgeV2(context.Background(), txToUnpledge, did, tx); err != nil {
			c.log.Error("CallBackQuorumUnpledge: UnpledgeV2 failed",
				"prevTxID", txToUnpledge,
				"did", did,
				"err", err,
			)
		}
	}

	return nil
}
