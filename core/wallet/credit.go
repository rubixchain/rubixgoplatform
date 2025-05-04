package wallet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// TODO: Credit structure needs to be worked upon
type Credit struct {
	DID    string `gorm:"column:did"`
	Credit string `gorm:"column:credit;size:4000"`
	Tx     string `gorm:"column:tx"`
}

// TODO: Credit structure needs to be worked upon
type PledgeInformation struct {
	TokenID         string `json:"token_id"`
	TokenType       int    `json:"token_type"`
	PledgeBlockID   string `json:"pledge_block_id"`
	UnpledgeBlockID string `json:"unpledge_block_id"`
	QuorumDID       string `json:"quorum_did"`
	TransactionID   string `json:"transaction_id"`
}

func (w *Wallet) AddPledgeHistory(pledgeDetails []model.PledgeHistory) error {
	for _, detail := range pledgeDetails {
		err := w.s.Write(PledgeHistoryTable, &detail)
		if err != nil {
			w.log.Error("Failed to add pledge details to pledge history table", "err", err)
			return err
		}
	}
	return nil
}

func (w *Wallet) CheckTokenExistInPledgeHistory(tokenID string, transID string) (bool, error) {
	var existingPledgeHistory model.PledgeHistory
	pledgeHistoryReadErr := w.s.Read(PledgeHistoryTable, &existingPledgeHistory, "transfer_tokens_id=? AND transaction_id=?", tokenID, transID)
	if pledgeHistoryReadErr != nil {
		readErr := fmt.Sprint(pledgeHistoryReadErr)
		if strings.Contains(readErr, "no records found") {
			w.log.Info("No pledge history")
			return false, pledgeHistoryReadErr
		}
		w.log.Error("Failed to read pledge history", "err", pledgeHistoryReadErr)
		return false, pledgeHistoryReadErr
	} else {
		return true, nil
	}
}

// func (w *Wallet) GetTokenDetailsByQuorumDID(quorumDID string) (map[string]TokenInfo, error) {
// 	var pledges []PledgeHistory
// 	tokenDetails := make(map[string]TokenInfo)

// 	// Query the database for records matching the given QuorumDID
// 	err := w.s.Read(PledgeHistoryTable, &pledges, "quorum_did=? and token_credit_status=?", quorumDID, 0)
// 	if err != nil {
// 		if strings.Contains(fmt.Sprint(err), "no records found") {
// 			w.log.Info("No pledge history found for given QuorumDID", "quorumDID", quorumDID)
// 			return nil, nil // Return nil if no records are found
// 		}
// 		w.log.Error("Failed to read pledge history", "quorumDID", quorumDID, "err", err)
// 		return nil, err
// 	}

// 	// Iterate over the results and collect token details
// 	for _, pledge := range pledges {
// 		tokenDetails[pledge.TransferTokenID] = TokenInfo{
// 			TokenType:           pledge.TransferTokenType,
// 			TransferBlockNumber: uint64(pledge.CurrentBlockNumber),
// 			TransactionID:       pledge.TransactionID,
// 			TransactionEpoch:    pledge.Epoch,
// 			TransferBlockID:     pledge.TransferBlockID,
// 		}
// 	}

//		return tokenDetails, nil
//	}
// func (w *Wallet) GetTokenDetailsByQuorumDID(quorumDID string, tokenCreditStatus int) (map[string][]TokenInfo, error) {
// 	var pledges []PledgeHistory
// 	tokenDetails := make(map[string][]TokenInfo) // Map with slice of TokenInfo

// 	// Query the database for records matching the given QuorumDID
// 	err := w.s.Read(PledgeHistoryTable, &pledges, "quorum_did=? and token_credit_status=?", quorumDID, tokenCreditStatus)
// 	if err != nil {
// 		if strings.Contains(fmt.Sprint(err), "no records found") {
// 			w.log.Info("No pledge history found for given QuorumDID", "quorumDID", quorumDID)
// 			return nil, nil // Return nil if no records are found
// 		}
// 		w.log.Error("Failed to read pledge history", "quorumDID", quorumDID, "err", err)
// 		return nil, err
// 	}

// 	// Iterate over the results and group TokenInfo by TransferTokenID
// 	for _, pledge := range pledges {
// 		tokenInfo := TokenInfo{
// 			TokenType:            pledge.TransferTokenType,
// 			TransactionID:        pledge.TransactionID,
// 			TransactionEpoch:     pledge.Epoch,
// 			TransferBlockID:      pledge.TransferBlockID,
// 			TransTokenValue:      pledge.TransferTokenValue,
// 			TokenCredit:          pledge.TokenCredit,
// 			TokenCreditStatus:    pledge.TokenCreditStatus,
// 			LatestTokenStateHash: pledge.LatestTokenStateHash,
// 		}

// 		// Append to the slice of TokenInfo for this TransferTokenID
// 		tokenDetails[pledge.TransferTokenID] = append(tokenDetails[pledge.TransferTokenID], tokenInfo)
// 	}

// 	return tokenDetails, nil
// }

// New GetTokenDetailsByQuorumDid function below
func (w *Wallet) GetTokenDetailsByQuorumDID(quorumDID string, tokenCreditStatus int) ([]model.PledgeHistory, error) {
	var pledges []model.PledgeHistory

	// Query the database for records matching the given QuorumDID and TokenCreditStatus
	err := w.s.Read(PledgeHistoryTable, &pledges, "quorum_did=? and token_credit_status=?", quorumDID, tokenCreditStatus)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "no records found") {
			w.log.Info("No pledge history found for given QuorumDID", "quorumDID", quorumDID)
			return nil, nil // Return nil if no records are found
		}
		w.log.Error("Failed to read pledge history", "quorumDID", quorumDID, "err", err)
		return nil, err
	}
	// fmt.Println("pledges in GetTokenDetailsByQuorumDID function", pledges)
	// Return the filtered pledge history records
	return pledges, nil
}

// This function will collect the required credit details for mining from the entire pledge history and add the remaining credits if any pledge history credit is not completly used.
func CollectRequiredCredits(pledges []model.PledgeHistory, requiredCredits uint64) ([]model.PledgeHistory, []model.PledgeHistory, error) {
	// Preallocate slices based on expected size
	usedCredits := make([]model.PledgeHistory, 0, len(pledges))
	unusedCredits := make([]model.PledgeHistory, 0, len(pledges))
	var totalSelected uint64 = 0

	// Use index-based loop for better performance [4]
	for i := 0; i < len(pledges); i++ {
		if totalSelected >= requiredCredits {
			// Add remaining pledges and exit early
			unusedCredits = append(unusedCredits, pledges[i:]...)
			break
		}

		pledge := pledges[i]
		available := pledge.TokenCredit
		needed := requiredCredits - totalSelected
		var usedAmount uint64
		if available < needed {
			usedAmount = available
		} else {
			usedAmount = needed
		}
		// Create modified copy with remaining credits
		modifiedPledge := pledge
		modifiedPledge.TokenCredit = usedAmount
		modifiedPledge.TokenCreditStatus = 2
		modifiedPledge.RemainingCredits = available - usedAmount

		if usedAmount > 0 {
			usedCredits = append(usedCredits, modifiedPledge)
			totalSelected += usedAmount
		}

		// Handle partial usage
		if usedAmount < available {
			// Add remaining pledges and exit
			if i+1 < len(pledges) {
				unusedCredits = append(unusedCredits, pledges[i+1:]...)
			}
			break
		}
	}

	if totalSelected < requiredCredits {
		return nil, nil, fmt.Errorf("insufficient credits: available %d, required %d", totalSelected, requiredCredits)
	}

	return usedCredits, unusedCredits, nil
}

func (w *Wallet) UpdateTokenCreditStatus(tokenID string, status int, transactionID string) error {
	var pledgeHistoryRecords []model.PledgeHistory

	err := w.s.Read(
		PledgeHistoryTable,
		&pledgeHistoryRecords,
		"transfer_tokens_id = ? and transaction_id=?",
		tokenID, transactionID,
	)
	if err != nil {
		fmt.Println("Error reading pledge history records:", err)
		return err
	}
	// Check if records exist
	if len(pledgeHistoryRecords) == 0 {
		fmt.Println("No records found for tokenID:", tokenID)
		return fmt.Errorf("no records found for tokenID: %s", tokenID)
	}

	// Update token_credit_status for each row
	for _, record := range pledgeHistoryRecords {
		record.TokenCreditStatus = status
		updateErr := w.s.Update(
			PledgeHistoryTable,
			record,
			"transfer_tokens_id = ? and transaction_id=?",
			tokenID, record.TransactionID,
		)
		if updateErr != nil {
			fmt.Println("Update failed for transaction:", record.TransactionID, "Error:", updateErr)
			continue // Continue updating other rows even if one fails
		}

		fmt.Println("Updated token_credit_status for transaction:", record.TransactionID)
	}

	return nil // Return nil if function completes successfully
}

// This function will return the list of tokensIDs from `PledgeHistoryTable` whose next block is not added
func (w *Wallet) GetTokenIDsWithoutNextBlockFromPledgeHistory() ([]string, error) {
	var pledgeDetails []model.PledgeHistory

	err := w.s.Read(PledgeHistoryTable, &pledgeDetails, "next_epoch = ? AND token_credit = ?", 0, 0)
	if err != nil {
		return nil, err
	}

	tokenIDs := make([]string, 0)
	for _, pledge := range pledgeDetails {
		if pledge.TransferTokenID != "" {
			tokenIDs = append(tokenIDs, pledge.TransferTokenID)
		}
	}
	return tokenIDs, nil
}

func (w *Wallet) UpdateEpochAndCreditInPledgeHistoryTable(tokenID string, transactionID string, transactionType int, epoch uint64, tokenType int) error {
	var pledgeHistoryRecords []model.PledgeHistory
	err := w.s.Read(PledgeHistoryTable, &pledgeHistoryRecords, "transfer_tokens_id = ? and transaction_id=?", tokenID, transactionID)
	if err != nil {
		fmt.Println("Error reading pledge history records:", err)
		return err
	}
	if len(pledgeHistoryRecords) == 0 {
		w.log.Error("no records found in Pledge history table for the token:", tokenID)
		return fmt.Errorf("no records found in Pledge history table for the token")
	}
	for _, record := range pledgeHistoryRecords {
		record.NextBlockEpoch = epoch

		// Fetch the latest block for the token
		latestBlock := w.GetLatestTokenBlock(record.TransferTokenID, tokenType)

		if record.TransactionType == 1 {
			var tokenCreditFloat float64
			if latestBlock != nil && latestBlock.GetMinerDID() != "" {
				// Mining quorum: 4x credits
				tokenCreditFloat = float64(epoch-record.Epoch) * float64(record.TransferTokenValue) * 15 * 4
			} else {
				// Normal quorum
				tokenCreditFloat = float64(epoch-record.Epoch) * float64(record.TransferTokenValue) * 15
			}
			tokenCreditsForEachQuorum := tokenCreditFloat / 5
			record.TokenCredit = uint64(tokenCreditsForEachQuorum)

		} else if record.TransactionType == 2 {
			var tokenCreditFloat float64
			if latestBlock != nil && latestBlock.GetMinerDID() != "" {
				// Mining quorum: 4x credits
				tokenCreditFloat = float64(epoch-record.Epoch) * float64(record.TransferTokenValue) * 4
			} else {
				// Normal quorum
				tokenCreditFloat = float64(epoch-record.Epoch) * float64(record.TransferTokenValue)
			}
			tokenCreditsForEachQuorum := tokenCreditFloat / 5
			record.TokenCredit = uint64(tokenCreditsForEachQuorum)
		}
		updateErr := w.s.Update(PledgeHistoryTable, record, "transfer_tokens_id = ? and transaction_id=?", record.TransferTokenID, record.TransactionID)
		if updateErr != nil {
			fmt.Println("Epoch updation failed for token: ", record.TransferTokenID, "Error:", updateErr)
			continue // Continue updating other rows even if one fails
		}
	}
	return nil
}

func (w *Wallet) StoreCredit(transactionID string, quorumDID string, pledgeInfo []*PledgeInformation) error {
	pledgeInfoBytes, err := json.Marshal(pledgeInfo)
	if err != nil {
		return fmt.Errorf("failed while marshalling credits: %v", err.Error())
	}
	pledgeInfoEncoded := base64.StdEncoding.EncodeToString(pledgeInfoBytes)

	credit := &Credit{
		DID:    quorumDID,
		Credit: pledgeInfoEncoded,
		Tx:     transactionID,
	}

	return w.s.Write(CreditStorage, credit)
}

func (w *Wallet) GetCredit(did string) ([]string, error) {
	var c []Credit
	err := w.s.Read(CreditStorage, &c, "did=?", did)
	if err != nil {
		return nil, err
	}
	str := make([]string, 0)
	for i := range c {
		str = append(str, c[i].Credit)
	}
	return str, nil
}

func (w *Wallet) RemoveCredit(transactionID string) error {
	err := w.s.Delete(CreditStorage, &Credit{}, "tx = ?", transactionID)
	if err != nil {
		errMsg := fmt.Errorf("failed to remove Credit for transaction: %v", transactionID)
		w.log.Error(errMsg.Error())
		return errMsg
	}

	return nil
}
