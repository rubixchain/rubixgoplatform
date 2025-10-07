package wallet

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// RecordFailedFTDownload records a failed FT download for later retry
func (w *Wallet) RecordFailedFTDownload(transactionID, tokenID, ftName, senderDID, receiverDID, tokenPath, errorMsg string) error {
	failedDownload := &model.FailedFTDownload{
		TransactionID: transactionID,
		TokenID:       tokenID,
		FTName:        ftName,
		SenderDID:     senderDID,
		ReceiverDID:   receiverDID,
		TokenPath:     tokenPath,
		ErrorMessage:  errorMsg,
		Status:        model.FailedFTStatusPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	return w.s.Write(FailedFTDownloadStorage, failedDownload)
}

// GetFailedFTDownloads retrieves all failed FT downloads for a receiver
func (w *Wallet) GetFailedFTDownloads(receiverDID string, status string) ([]*model.FailedFTDownload, error) {
	var downloads []*model.FailedFTDownload
	
	query := "receiver_did=?"
	args := []interface{}{receiverDID}
	
	if status != "" {
		query += " AND status=?"
		args = append(args, status)
	}
	
	err := w.s.Read(FailedFTDownloadStorage, &downloads, query, args...)
	if err != nil && err.Error() != "no records found" {
		return nil, err
	}
	
	return downloads, nil
}

// GetFailedFTDownloadsByTransaction retrieves failed downloads for a specific transaction
func (w *Wallet) GetFailedFTDownloadsByTransaction(transactionID string) ([]*model.FailedFTDownload, error) {
	var downloads []*model.FailedFTDownload
	
	err := w.s.Read(FailedFTDownloadStorage, &downloads, "transaction_id=?", transactionID)
	if err != nil && err.Error() != "no records found" {
		return nil, err
	}
	
	return downloads, nil
}

// UpdateFailedFTDownloadStatus updates the status of a failed download
func (w *Wallet) UpdateFailedFTDownloadStatus(tokenID string, status string, errorMsg string) error {
	// First read the current record to increment retry count
	var download model.FailedFTDownload
	err := w.s.Read(FailedFTDownloadStorage, &download, "token_id=?", tokenID)
	if err != nil {
		return err
	}
	
	download.Status = status
	download.UpdatedAt = time.Now()
	download.RetryCount++
	
	if status == model.FailedFTStatusRetrying {
		now := time.Now()
		download.LastRetryAt = &now
	}
	
	if errorMsg != "" {
		download.ErrorMessage = errorMsg
	}
	
	return w.s.Update(FailedFTDownloadStorage, &download, "token_id=?", tokenID)
}

// MarkFailedFTDownloadAsRecovered marks a failed download as successfully recovered
func (w *Wallet) MarkFailedFTDownloadAsRecovered(tokenID string) error {
	return w.UpdateFailedFTDownloadStatus(tokenID, model.FailedFTStatusRecovered, "")
}

// GetFailedFTDownloadStats gets statistics about failed downloads
func (w *Wallet) GetFailedFTDownloadStats(receiverDID string) (map[string]int, error) {
	stats := make(map[string]int)
	
	// Get counts by status
	statuses := []string{
		model.FailedFTStatusPending,
		model.FailedFTStatusRetrying,
		model.FailedFTStatusRecovered,
		model.FailedFTStatusFailed,
	}
	
	for _, status := range statuses {
		var downloads []*model.FailedFTDownload
		err := w.s.Read(FailedFTDownloadStorage, &downloads, 
			"receiver_did=? AND status=?", receiverDID, status)
		if err != nil && err.Error() != "no records found" {
			return nil, err
		}
		stats[status] = len(downloads)
	}
	
	// Get total count
	var allDownloads []*model.FailedFTDownload
	err := w.s.Read(FailedFTDownloadStorage, &allDownloads, "receiver_did=?", receiverDID)
	if err != nil && err.Error() != "no records found" {
		return nil, err
	}
	stats["total"] = len(allDownloads)
	
	return stats, nil
}

// CleanupOldFailedDownloads removes old recovered or permanently failed downloads
func (w *Wallet) CleanupOldFailedDownloads(daysOld int) error {
	cutoffTime := time.Now().AddDate(0, 0, -daysOld)
	
	return w.s.Delete(FailedFTDownloadStorage, nil, 
		"(status = ? OR status = ?) AND updated_at < ?",
		model.FailedFTStatusRecovered, model.FailedFTStatusFailed, cutoffTime)
}