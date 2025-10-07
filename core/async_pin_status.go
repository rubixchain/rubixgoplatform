package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// AsyncPinStatusRequest represents a request to check pin status
type AsyncPinStatusRequest struct {
	TransactionID string `json:"transaction_id"`
}

// AsyncPinStatusResponse represents the pin status response
type AsyncPinStatusResponse struct {
	Status         string  `json:"status"`
	TransactionID  string  `json:"transaction_id"`
	TokenCount     int     `json:"token_count"`
	CompletedCount int32   `json:"completed_count"`
	FailedCount    int32   `json:"failed_count"`
	Progress       float64 `json:"progress_percentage"`
	StartTime      string  `json:"start_time"`
	Duration       string  `json:"duration,omitempty"`
	Error          string  `json:"error,omitempty"`
}

// GetAsyncPinStatus returns the status of an async pinning job
func (c *Core) GetAsyncPinStatus(req *ensweb.Request) *ensweb.Result {
	var statusReq AsyncPinStatusRequest
	err := c.l.ParseJSON(req, &statusReq)
	if err != nil {
		resp := model.BasicResponse{
			Status: false,
			Message: "Failed to parse request: " + err.Error(),
		}
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	if statusReq.TransactionID == "" {
		resp := model.BasicResponse{
			Status: false,
			Message: "Transaction ID is required",
		}
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	job, found := c.asyncPinManager.GetJobStatus(statusReq.TransactionID)
	if !found {
		resp := model.BasicResponse{
			Status: false,
			Message: "Pin job not found",
		}
		return c.l.RenderJSON(req, &resp, http.StatusOK)
	}

	// Calculate progress
	progress := float64(0)
	if job.TokenCount > 0 {
		completed := job.CompletedCount + job.FailedCount
		progress = float64(completed) / float64(job.TokenCount) * 100
	}

	// Status string
	statusStr := "unknown"
	switch job.Status {
	case PinStatusQueued:
		statusStr = "queued"
	case PinStatusInProgress:
		statusStr = "in_progress"
	case PinStatusCompleted:
		statusStr = "completed"
	case PinStatusFailed:
		statusStr = "failed"
	}

	response := AsyncPinStatusResponse{
		Status:         statusStr,
		TransactionID:  job.TransactionID,
		TokenCount:     job.TokenCount,
		CompletedCount: job.CompletedCount,
		FailedCount:    job.FailedCount,
		Progress:       progress,
		StartTime:      job.StartTime.Format(time.RFC3339),
	}

	// Add duration if job is completed or failed
	if job.Status == PinStatusCompleted || job.Status == PinStatusFailed {
		response.Duration = time.Since(job.StartTime).String()
	}

	// Add error if present
	if job.Error != nil {
		response.Error = job.Error.Error()
	}

	return c.l.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: "Pin status retrieved",
		Result:  response,
	}, http.StatusOK)
}

// GetAllAsyncPinJobs returns all async pin jobs
func (c *Core) GetAllAsyncPinJobs(req *ensweb.Request) *ensweb.Result {
	var jobs []AsyncPinStatusResponse

	c.asyncPinManager.jobs.Range(func(key, value interface{}) bool {
		job := value.(*AsyncPinJob)
		
		// Calculate progress
		progress := float64(0)
		if job.TokenCount > 0 {
			completed := job.CompletedCount + job.FailedCount
			progress = float64(completed) / float64(job.TokenCount) * 100
		}

		// Status string
		statusStr := "unknown"
		switch job.Status {
		case PinStatusQueued:
			statusStr = "queued"
		case PinStatusInProgress:
			statusStr = "in_progress"
		case PinStatusCompleted:
			statusStr = "completed"
		case PinStatusFailed:
			statusStr = "failed"
		}

		response := AsyncPinStatusResponse{
			Status:         statusStr,
			TransactionID:  job.TransactionID,
			TokenCount:     job.TokenCount,
			CompletedCount: job.CompletedCount,
			FailedCount:    job.FailedCount,
			Progress:       progress,
			StartTime:      job.StartTime.Format(time.RFC3339),
		}

		// Add duration if job is completed or failed
		if job.Status == PinStatusCompleted || job.Status == PinStatusFailed {
			response.Duration = time.Since(job.StartTime).String()
		}

		// Add error if present
		if job.Error != nil {
			response.Error = job.Error.Error()
		}

		jobs = append(jobs, response)
		return true
	})

	return c.l.RenderJSON(req, &model.BasicResponse{
		Status:  true,
		Message: fmt.Sprintf("Found %d pin jobs", len(jobs)),
		Result:  jobs,
	}, http.StatusOK)
}

// CleanupAsyncPinJobs cleans up old completed/failed jobs
func (c *Core) CleanupAsyncPinJobs(req *ensweb.Request) *ensweb.Result {
	// Clean up jobs older than 1 hour
	cleaned := c.asyncPinManager.CleanupOldJobs(1 * time.Hour)
	
	resp := model.BasicResponse{
		Status: true,
		Message: fmt.Sprintf("Cleaned up %d old pin jobs", cleaned),
	}
	return c.l.RenderJSON(req, &resp, http.StatusOK)
}