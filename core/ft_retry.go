package core

import (
	"fmt"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

// RetryFailedFTDownloads retries all pending failed FT downloads for a receiver
func (c *Core) RetryFailedFTDownloads(receiverDID string) (string, error) {
	c.log.Info("Starting retry of failed FT downloads", "receiver_did", receiverDID)
	
	// Get all pending failed downloads
	failedDownloads, err := c.w.GetFailedFTDownloads(receiverDID, model.FailedFTStatusPending)
	if err != nil {
		c.log.Error("Failed to get failed downloads", "err", err)
		return "", fmt.Errorf("failed to get failed downloads: %w", err)
	}
	
	if len(failedDownloads) == 0 {
		c.log.Info("No failed downloads to retry")
		return "No failed downloads to retry", nil
	}
	
	c.log.Info("Found failed downloads to retry", "count", len(failedDownloads))
	
	// Group by transaction ID for efficient processing
	downloadsByTxn := make(map[string][]*model.FailedFTDownload)
	for _, download := range failedDownloads {
		downloadsByTxn[download.TransactionID] = append(downloadsByTxn[download.TransactionID], download)
	}
	
	totalRetried := 0
	totalRecovered := 0
	totalFailed := 0
	
	// Process each transaction's failed downloads
	for txnID, downloads := range downloadsByTxn {
		c.log.Info("Retrying downloads for transaction", "txn_id", txnID, "count", len(downloads))
		
		recovered, failed := c.retryTransactionDownloads(downloads)
		totalRecovered += recovered
		totalFailed += failed
		totalRetried += len(downloads)
	}
	
	// Get final statistics
	stats, err := c.w.GetFailedFTDownloadStats(receiverDID)
	if err != nil {
		c.log.Error("Failed to get download stats", "err", err)
	}
	
	summary := fmt.Sprintf(
		"Retry completed: %d attempted, %d recovered, %d failed. Current status: %d pending, %d recovered, %d failed",
		totalRetried, totalRecovered, totalFailed,
		stats[model.FailedFTStatusPending],
		stats[model.FailedFTStatusRecovered],
		stats[model.FailedFTStatusFailed],
	)
	
	c.log.Info(summary)
	return summary, nil
}

// retryTransactionDownloads retries failed downloads for a specific transaction
func (c *Core) retryTransactionDownloads(downloads []*model.FailedFTDownload) (recovered, failed int) {
	for _, download := range downloads {
		// Update status to retrying
		err := c.w.UpdateFailedFTDownloadStatus(download.TokenID, model.FailedFTStatusRetrying, "")
		if err != nil {
			c.log.Error("Failed to update retry status", "token_id", download.TokenID, "err", err)
			continue
		}
		
		// Attempt to download the token
		err = c.retrySingleTokenDownload(download)
		if err != nil {
			c.log.Error("Failed to retry download", 
				"token_id", download.TokenID,
				"retry_count", download.RetryCount+1,
				"err", err)
			
			// Update status based on retry count
			status := model.FailedFTStatusPending
			if download.RetryCount >= 2 { // After 3 total attempts, mark as permanently failed
				status = model.FailedFTStatusFailed
			}
			
			c.w.UpdateFailedFTDownloadStatus(download.TokenID, status, err.Error())
			failed++
		} else {
			c.log.Info("Successfully recovered token", 
				"token_id", download.TokenID,
				"retry_count", download.RetryCount+1)
			
			// Mark as recovered
			c.w.MarkFailedFTDownloadAsRecovered(download.TokenID)
			recovered++
		}
	}
	
	return recovered, failed
}

// retrySingleTokenDownload attempts to download and store a single failed token
func (c *Core) retrySingleTokenDownload(download *model.FailedFTDownload) error {
	// Get the token from IPFS
	startTime := time.Now()
	
	// Use the wallet's IPFS operations to download
	err := c.w.GetIpfsOps().Get(download.TokenID, download.TokenPath)
	if err != nil {
		return fmt.Errorf("IPFS download failed: %w", err)
	}
	
	c.log.Debug("Token downloaded successfully", 
		"token_id", download.TokenID,
		"duration", time.Since(startTime))
	
	// Read the existing FT token record to check if it's already marked as free
	var ftToken wallet.FTToken
	err = c.w.GetStorage().Read(wallet.FTTokenStorage, &ftToken, "token_id=?", download.TokenID)
	if err != nil {
		// If token doesn't exist, we need to create it
		// This is more complex and might need transaction context
		return fmt.Errorf("token record not found: %w", err)
	}
	
	// Update token status if it's still pending
	if ftToken.TokenStatus == wallet.TokenIsPending {
		ftToken.TokenStatus = wallet.TokenIsFree
		ftToken.UpdatedAt = time.Now()
		
		err = c.w.GetStorage().Update(wallet.FTTokenStorage, &ftToken, "token_id=?", download.TokenID)
		if err != nil {
			return fmt.Errorf("failed to update token status: %w", err)
		}
	}
	
	return nil
}

// GetFailedFTDownloadStatus returns the status of failed downloads for a receiver
func (c *Core) GetFailedFTDownloadStatus(receiverDID string) (map[string]interface{}, error) {
	stats, err := c.w.GetFailedFTDownloadStats(receiverDID)
	if err != nil {
		return nil, fmt.Errorf("failed to get download stats: %w", err)
	}
	
	// Get recent failures
	recentFailures, err := c.w.GetFailedFTDownloads(receiverDID, model.FailedFTStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent failures: %w", err)
	}
	
	// Create summary of recent failures by transaction
	failuresByTxn := make(map[string]int)
	for _, failure := range recentFailures {
		failuresByTxn[failure.TransactionID]++
	}
	
	result := map[string]interface{}{
		"statistics": stats,
		"recent_failures_by_transaction": failuresByTxn,
		"total_pending": stats[model.FailedFTStatusPending],
		"can_retry": stats[model.FailedFTStatusPending] > 0,
	}
	
	return result, nil
}

// CleanupFailedFTDownloads removes old recovered or failed download records
func (c *Core) CleanupFailedFTDownloads(daysOld int) (string, error) {
	err := c.w.CleanupOldFailedDownloads(daysOld)
	if err != nil {
		return "", fmt.Errorf("failed to cleanup old downloads: %w", err)
	}
	
	return fmt.Sprintf("Cleaned up download records older than %d days", daysOld), nil
}