package core

import "github.com/rubixchain/rubixgoplatform/types/models"

// Quorums listening on "rubix_txns" event will check the `Tokens` attribute
// of incoming `TransactionInfo` (recieved via main Callback function of "rubix_txns")
// and will go through all Tokens. Quorum check if the TokenStateHash is present
// in their DB or not (for which they have pledged). If they have, then they can
// remove all the records from the TokenStateHash table
func (c *Core) CallBackQuorumUnpledge(tx models.TransactionInfo) error {
	// Step 1: Loop through all the tokens and gather transactionID and
	// pledge Token ID map
	var prevTransactionsSet map[string]struct{} = make(map[string]struct{})

	addToSet := func(id string) {
		if id != "" {
			prevTransactionsSet[id] = struct{}{}
		}
	}

	for _, rbtToken := range tx.Tokens.RBT {
		addToSet(rbtToken.PreviousTransactionID)
	}

	for _, ftToken := range tx.Tokens.FT {
		addToSet(ftToken.PreviousTransactionID)
	}

	for _, nftToken := range tx.Tokens.NFT {
		addToSet(nftToken.PreviousTransactionID)
	}

	for _, smartContractToken := range tx.Tokens.SmartContract {
		addToSet(smartContractToken.PreviousTransactionID)
	}

	if len(prevTransactionsSet) == 0 {
		return nil
	}

	var prevTransactionList []string = make([]string, 0, len(prevTransactionsSet))
	for prevTransaction := range prevTransactionsSet {
		prevTransactionList = append(prevTransactionList, prevTransaction)
	}

	if err := c.w.RemoveTokenStateHashes(prevTransactionList); err != nil {
		return err
	}

	return nil
}