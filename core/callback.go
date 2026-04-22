package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
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

	// The following list is maintained to keep a list of parent tokens for the corresponding
	// part tokens present in `txInfo.Tokens.RBT` attribute. This is done to cover the scenario
	// where the parent token, with a lineage of transfer history, was burnt to mint that the said part token.
	// In such cases, any quorum who pledged for that parent token should be able to unpledge.
	var rbtParentTokenList []string = make([]string, 0)

	// Loop through all the tokens and gather previous transactionID and
	// pledge Token ID map
	var prevTransactionsSet map[string]struct{} = make(map[string]struct{})
	var transactionTokens map[string][]string = make(map[string][]string)

	addToSet := func(id string) {
		if id != "" {
			prevTransactionsSet[id] = struct{}{}
		}
	}

	for _, rbtToken := range txInfo.Tokens.RBT {
		addToSet(rbtToken.PreviousTransactionID)

		if rbtToken.PreviousTransactionID != "" {
			if _, ok := transactionTokens[rbtToken.PreviousTransactionID]; !ok {
				transactionTokens[rbtToken.PreviousTransactionID] = make([]string, 0)
			}
			transactionTokens[rbtToken.PreviousTransactionID] = append(transactionTokens[rbtToken.PreviousTransactionID], rbtToken.TokenID)
		}

		// If the RBT token has a parent token, add it to the parentTokenList
		tokenValue, err := util.GetTokenValueFromTokenID(rbtToken.TokenID)
		if err != nil {
			return fmt.Errorf("CallBackQuorumUnpledge: failed to get token value for RBT token %q, err: %v", rbtToken.TokenID, err)
		}
		if tokenValue != rubixmath.OneFloat() {
			ancestorTokens, err := util.TokenID(rbtToken.TokenID).GetHierarchy()
			if err != nil {
				return fmt.Errorf("CallBackQuorumUnpledge: failed to get parent token for RBT token %q, err: %v", rbtToken.TokenID, err)
			}
			rbtParentTokenList = append(rbtParentTokenList, ancestorTokens...)
		}
	}

	for _, ftToken := range txInfo.Tokens.FT {
		addToSet(ftToken.PreviousTransactionID)
		
		if ftToken.PreviousTransactionID != "" {
			if _, ok := transactionTokens[ftToken.PreviousTransactionID]; !ok {
				transactionTokens[ftToken.PreviousTransactionID] = make([]string, 0)
			}
			transactionTokens[ftToken.PreviousTransactionID] = append(transactionTokens[ftToken.PreviousTransactionID], ftToken.TokenID)
		}
	}

	for _, nftToken := range txInfo.Tokens.NFT {
		addToSet(nftToken.PreviousTransactionID)

		if nftToken.PreviousTransactionID != "" {
			if _, ok := transactionTokens[nftToken.PreviousTransactionID]; !ok {
				transactionTokens[nftToken.PreviousTransactionID] = make([]string, 0)
			}
			transactionTokens[nftToken.PreviousTransactionID] = append(transactionTokens[nftToken.PreviousTransactionID], nftToken.TokenID)
		}
	}

	for _, smartContractToken := range txInfo.Tokens.SmartContract {
		addToSet(smartContractToken.PreviousTransactionID)

		if smartContractToken.PreviousTransactionID != "" {
			if _, ok := transactionTokens[smartContractToken.PreviousTransactionID]; !ok {
				transactionTokens[smartContractToken.PreviousTransactionID] = make([]string, 0)
			}
			transactionTokens[smartContractToken.PreviousTransactionID] = append(transactionTokens[smartContractToken.PreviousTransactionID], smartContractToken.TokenID)
		}
	}

	for _, committedToken := range txInfo.CommittedTokens {
		addToSet(committedToken.PreviousTransactionID)

		if committedToken.PreviousTransactionID != "" {
			if _, ok := transactionTokens[committedToken.PreviousTransactionID]; !ok {
				transactionTokens[committedToken.PreviousTransactionID] = make([]string, 0)
			}
			transactionTokens[committedToken.PreviousTransactionID] = append(transactionTokens[committedToken.PreviousTransactionID], committedToken.TokenID)
		}
	}

	if len(prevTransactionsSet) == 0 && len(rbtParentTokenList) == 0 {
		return nil
	}

	var prevTransactionList []string = make([]string, 0, len(prevTransactionsSet))
	for prevTransaction := range prevTransactionsSet {
		prevTransactionList = append(prevTransactionList, prevTransaction)
	}

	transactionToUnpledge, err := c.w.CheckTxnsPresentInUnpledgeSequenceInfo(prevTransactionList, did, transactionTokens, rbtParentTokenList)
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
