package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// ServeDashboard serves the dashboard HTML and assets
func (s *Server) ServeDashboard(req *ensweb.Request) *ensweb.Result {
	// Determine the path based on the request
	urlPath := req.Path
	if urlPath == "/dashboard" || urlPath == "/dashboard/" {
		urlPath = "/dashboard/index.html"
	} else if urlPath == "/dashboard.js" {
		urlPath = "/dashboard/dashboard.js"
	}

	// Construct the file path
	basePath := filepath.Join(".", "dashboard")
	var requestedFile string

	if urlPath == "/dashboard/index.html" {
		requestedFile = filepath.Join(basePath, "index.html")
	} else if strings.HasPrefix(urlPath, "/dashboard/") {
		requestedFile = filepath.Join(basePath, filepath.Clean(urlPath[len("/dashboard/"):]))
	} else {
		requestedFile = filepath.Join(basePath, "index.html")
	}

	// Check if file exists
	info, err := os.Stat(requestedFile)
	if err != nil {
		// If file doesn't exist, serve embedded content as fallback
		if urlPath == "/dashboard/index.html" || urlPath == "/dashboard" || urlPath == "/dashboard/" {
			// Create temporary HTML file
			tmpFile := filepath.Join(os.TempDir(), "dashboard.html")
			os.WriteFile(tmpFile, []byte(getEmbeddedDashboardHTML()), 0644)
			return s.RenderFile(req, tmpFile, false)
		} else if urlPath == "/dashboard/dashboard.js" {
			// Create temporary JS file with proper extension
			tmpFile := filepath.Join(os.TempDir(), "dashboard.js")
			os.WriteFile(tmpFile, []byte(getEmbeddedDashboardJS()), 0644)
			return s.RenderFile(req, tmpFile, false)
		}
		return s.BasicResponse(req, false, "File not found", nil)
	}

	// Don't serve directories
	if info.IsDir() {
		return s.BasicResponse(req, false, "Not found", nil)
	}

	// For JavaScript files, use a custom approach to set the proper MIME type
	if strings.HasSuffix(requestedFile, ".js") {
		// Use the extension mapping to ensure proper MIME type
		s.AddExtension(".js", "application/javascript")
	}

	// Use RenderFile for existing files - it handles MIME types properly
	return s.RenderFile(req, requestedFile, false)
}

// GetDashboardData returns dashboard data as JSON
func (s *Server) GetDashboardData(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	// Get pledged tokens count
	pledgedTokens := getPledgedTokensData(s, did)
	pledgedCount := len(pledgedTokens)

	// Get account info
	accountInfo, err := s.c.GetAccountInfo(did)
	if err != nil {
		s.log.Error("Failed to get account info", "did", did, "err", err)
		// Return partial data even if account info fails (use 0 balance as fallback)
		availableTokens := 0.0 // No balance available
		totalTokens := availableTokens + float64(pledgedCount)

		data := map[string]interface{}{
			"stats": map[string]interface{}{
				"did":              did,
				"peer_id":          s.c.GetPeerID(),
				"balance":          availableTokens,
				"pledged_tokens":   pledgedCount,
				"available_tokens": availableTokens,
				"total_tokens":     totalTokens,
				"last_updated":     time.Now(),
			},
			"pledged_tokens":      pledgedTokens,
			"unpledge_sequences":  getUnpledgeSequencesData(s, did),
			"recent_transactions": getRecentTransactionsData(s, did),
		}
		return s.RenderJSON(req, data, http.StatusOK)
	}

	// Calculate correct values based on your requirements:
	// Available Tokens = Balance (RBT Amount)
	// Total Tokens = Available Tokens + Pledged Tokens
	availableTokens := accountInfo.RBTAmount
	totalTokens := availableTokens + float64(pledgedCount)

	// Build dashboard data
	data := map[string]interface{}{
		"stats": map[string]interface{}{
			"did":              did,
			"peer_id":          s.c.GetPeerID(),
			"balance":          accountInfo.RBTAmount,
			"pledged_tokens":   pledgedCount,
			"available_tokens": availableTokens,
			"total_tokens":     totalTokens,
			"last_updated":     time.Now(),
		},
		"pledged_tokens":      pledgedTokens,
		"unpledge_sequences":  getUnpledgeSequencesData(s, did),
		"recent_transactions": getRecentTransactionsData(s, did),
	}

	return s.RenderJSON(req, data, http.StatusOK)
}

// GetDashboardPledgedTokens returns pledged tokens data
func (s *Server) GetDashboardPledgedTokens(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	pledgedTokens := getPledgedTokensData(s, did)

	data := map[string]interface{}{
		"status": true,
		"tokens": pledgedTokens,
	}

	return s.RenderJSON(req, data, http.StatusOK)
}

// GetDashboardUnpledgeSequence returns unpledge sequence data
func (s *Server) GetDashboardUnpledgeSequence(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	sequences := getUnpledgeSequencesData(s, did)

	data := map[string]interface{}{
		"status":    true,
		"sequences": sequences,
	}

	return s.RenderJSON(req, data, http.StatusOK)
}

// API endpoints for dashboard
func (s *Server) GetDashboardTokensAPI(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	tokens, err := s.c.GetTokensByDID(did)
	if err != nil {
		s.log.Error("Failed to get tokens", "did", did, "err", err)
		return s.RenderJSON(req, map[string]interface{}{
			"tokens": []interface{}{},
			"error":  err.Error(),
		}, http.StatusOK)
	}

	// If tokens is nil, return empty array instead
	if tokens == nil {
		tokens = []interface{}{}
	}

	return s.RenderJSON(req, map[string]interface{}{
		"tokens": tokens,
	}, http.StatusOK)
}

func (s *Server) GetDashboardAccountInfoAPI(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	accountInfo, err := s.c.GetAccountInfo(did)
	if err != nil {
		return s.BasicResponse(req, false, "Failed to get account info", err)
	}

	return s.RenderJSON(req, accountInfo, http.StatusOK)
}

func (s *Server) GetDashboardTransactionsAPI(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	txns, err := s.c.GetTransactionHistory(did, 100)
	if err != nil {
		return s.RenderJSON(req, map[string]interface{}{
			"transactions": []interface{}{},
			"error":        err.Error(),
		}, http.StatusOK)
	}

	// Ensure we return an array, not null
	if txns == nil {
		txns = []core.TransactionInfo{}
	}

	return s.RenderJSON(req, map[string]interface{}{
		"transactions": txns,
	}, http.StatusOK)
}

// GetDashboardDIDsAPI returns all available DIDs
func (s *Server) GetDashboardDIDsAPI(req *ensweb.Request) *ensweb.Result {
	dids := s.c.GetAllDIDs()

	return s.RenderJSON(req, map[string]interface{}{
		"dids": dids,
	}, http.StatusOK)
}

func (s *Server) GetDashboardUnpledgeSequenceAPI(req *ensweb.Request) *ensweb.Result {
	did := s.GetQuerry(req, "did")
	if did == "" {
		dids := s.c.GetAllDIDs()
		if len(dids) > 0 {
			did = dids[0]
		}
	}

	sequences, err := s.c.GetUnpledgeSequences(did)
	if err != nil {
		// Return empty array on error
		sequences = []core.UnpledgeSequenceInfo{}
	}

	// Ensure we return an array, not null
	if sequences == nil {
		sequences = []core.UnpledgeSequenceInfo{}
	}

	return s.RenderJSON(req, map[string]interface{}{
		"sequences": sequences,
	}, http.StatusOK)
}

// Helper functions
func getPledgedTokensData(s *Server, did string) []interface{} {
	pledgedTokens, err := s.c.GetPledgedTokens(did)
	if err != nil {
		return []interface{}{}
	}

	// Convert to interface array
	result := make([]interface{}, len(pledgedTokens))
	for i, token := range pledgedTokens {
		result[i] = token
	}
	return result
}

func getUnpledgeSequencesData(s *Server, did string) []interface{} {
	sequences, err := s.c.GetUnpledgeSequences(did)
	if err != nil {
		return []interface{}{}
	}

	// Convert to interface array
	result := make([]interface{}, len(sequences))
	for i, seq := range sequences {
		result[i] = seq
	}
	return result
}

func getRecentTransactionsData(s *Server, did string) []interface{} {
	txns, err := s.c.GetTransactionHistory(did, 50)
	if err != nil {
		return []interface{}{}
	}

	// Convert to interface array
	result := make([]interface{}, len(txns))
	for i, tx := range txns {
		result[i] = tx
	}
	return result
}

// Embedded dashboard HTML (fallback if file not found)
func getEmbeddedDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>RubixGo Token Dashboard</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 20px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { text-align: center; color: white; margin-bottom: 30px; }
        .error { background: #fee; color: #c00; padding: 20px; border-radius: 8px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>RubixGo Token Dashboard</h1>
        </div>
        <div class="error">
            Dashboard files not found. Please ensure dashboard files are in the ./dashboard directory.
        </div>
    </div>
</body>
</html>`
}

// Embedded dashboard JS (fallback if file not found)
func getEmbeddedDashboardJS() string {
	return `console.log('Dashboard JavaScript not found. Please ensure dashboard.js is in the ./dashboard directory.');`
}
