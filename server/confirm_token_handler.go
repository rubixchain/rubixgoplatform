package server

import (
	"github.com/rubixchain/rubixgoplatform/core"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// APIConfirmTokenTransfer handles the CRITICAL missing endpoint for receiver confirmation
// This endpoint is called by receivers to confirm they have successfully received tokens
// WITHOUT THIS, THE ENTIRE TWO-PHASE COMMIT FAILS!
func (s *Server) APIConfirmTokenTransfer(req *ensweb.Request) *ensweb.Result {
	// Parse the confirmation request from receiver - following pattern from other handlers
	ctr := core.ConfirmTokenRequest{}
	
	// Access data from the request using the Data map (standard pattern in this codebase)
	if req.Data != nil {
		if txID, ok := req.Data["transaction_id"].(string); ok {
			ctr.TransactionID = txID
		}
		if tokens, ok := req.Data["tokens"].([]interface{}); ok {
			ctr.Tokens = make([]string, 0, len(tokens))
			for _, token := range tokens {
				if tokenStr, ok := token.(string); ok {
					ctr.Tokens = append(ctr.Tokens, tokenStr)
				}
			}
		}
		if tokenType, ok := req.Data["token_type"].(float64); ok {
			ctr.TokenType = int(tokenType)
		}
	}

	// Validate required fields
	if ctr.TransactionID == "" {
		return s.BasicResponse(req, false, "Transaction ID is required", nil)
	}
	if len(ctr.Tokens) == 0 {
		return s.BasicResponse(req, false, "Token list is required", nil)
	}

	s.log.Info("Processing token confirmation from receiver",
		"transaction_id", ctr.TransactionID,
		"token_count", len(ctr.Tokens),
		"token_type", ctr.TokenType)

	// Process the confirmation
	err := s.c.APIConfirmTokenTransfer(&ctr)
	if err != nil {
		s.log.Error("Failed to process token confirmation",
			"transaction_id", ctr.TransactionID,
			"error", err)
		return s.BasicResponse(req, false, err.Error(), nil)
	}

	s.log.Info("Token confirmation processed successfully",
		"transaction_id", ctr.TransactionID)

	return s.BasicResponse(req, true, "Confirmation received and processed", nil)
}