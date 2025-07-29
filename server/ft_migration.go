package server

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIMigrateFTTransactions triggers migration of historical FT transaction data
func (s *Server) APIMigrateFTTransactions(req *ensweb.Request) *ensweb.Result {
	// Check if migration is already running
	status, err := s.c.GetFTMigrationStatus()
	if err == nil && status.Status == "running" {
		return s.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: "Migration already in progress",
			Result: map[string]interface{}{
				"started_at":    status.StartTime,
				"processed":     status.ProcessedCount,
				"total":         status.TotalTransactions,
			},
		}, 200)
	}
	
	// Start migration in background
	go func() {
		s.log.Info("Starting FT transaction migration in background")
		_, err := s.c.MigrateFTTransactionTokens()
		if err != nil {
			s.log.Error("FT transaction migration failed", "error", err)
		} else {
			s.log.Info("FT transaction migration completed successfully")
		}
	}()
	
	return s.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: "FT transaction migration started",
	}, 200)
}

// APIGetFTMigrationStatus returns the current or last migration status
func (s *Server) APIGetFTMigrationStatus(req *ensweb.Request) *ensweb.Result {
	status, err := s.c.GetFTMigrationStatus()
	if err != nil {
		return s.RenderJSON(req, &model.BasicResponse{
			Status:  false,
			Message: err.Error(),
		}, 200)
	}
	
	return s.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: "Migration status retrieved",
		Result: map[string]interface{}{
			"status":           status.Status,
			"start_time":       status.StartTime,
			"end_time":         status.EndTime,
			"total":            status.TotalTransactions,
			"processed":        status.ProcessedCount,
			"success":          status.SuccessCount,
			"failed":           status.FailureCount,
			"last_txn_id":      status.LastProcessedTxnID,
		},
	}, 200)
}