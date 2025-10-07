package server

import (
	"net/http"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// RetryFailedFTDownloadsRequest represents the request for retrying failed downloads
type RetryFailedFTDownloadsRequest struct {
	DID string `json:"did"`
}

// FailedFTDownloadStatusRequest represents the request for getting failed download status
type FailedFTDownloadStatusRequest struct {
	DID string `json:"did"`
}

// @Summary     Retry Failed FT Downloads
// @Description This API retries downloading failed FT tokens
// @Tags        FT
// @ID          retry-failed-ft-downloads
// @Accept      json
// @Produce     json
// @Param       input body RetryFailedFTDownloadsRequest true "DID"
// @Success     200 {object} model.BasicResponse
// @Router      /api/retry-failed-ft-downloads [post]
func (s *Server) RetryFailedFTDownloads(req *ensweb.Request) *ensweb.Result {
	var input RetryFailedFTDownloadsRequest
	err := s.ParseJSON(req, &input)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request: "+err.Error(), nil)
	}

	// Validate DID access if DID is provided
	if input.DID != "" {
		if !s.validateDIDAccess(req, input.DID) {
			return s.BasicResponse(req, false, "DID does not have access", nil)
		}
	}

	// Retry failed downloads
	result, err := s.c.RetryFailedFTDownloads(input.DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to retry downloads: "+err.Error(), nil)
	}

	return s.BasicResponse(req, true, result, nil)
}

// @Summary     Get Failed FT Download Status
// @Description This API returns the status of failed FT downloads
// @Tags        FT
// @ID          get-failed-ft-download-status
// @Accept      json
// @Produce     json
// @Param       input body FailedFTDownloadStatusRequest true "DID"
// @Success     200 {object} model.BasicResponse
// @Router      /api/get-failed-ft-download-status [post]
func (s *Server) GetFailedFTDownloadStatus(req *ensweb.Request) *ensweb.Result {
	var input FailedFTDownloadStatusRequest
	err := s.ParseJSON(req, &input)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid request: "+err.Error(), nil)
	}

	// Validate DID access if DID is provided
	if input.DID != "" {
		if !s.validateDIDAccess(req, input.DID) {
			return s.BasicResponse(req, false, "DID does not have access", nil)
		}
	}

	// Get failed download status
	status, err := s.c.GetFailedFTDownloadStatus(input.DID)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get status: "+err.Error(), nil)
	}

	resp := model.BasicResponse{
		Status:  true,
		Message: "Failed FT download status retrieved",
		Result:  status,
	}

	return s.RenderJSON(req, resp, http.StatusOK)
}