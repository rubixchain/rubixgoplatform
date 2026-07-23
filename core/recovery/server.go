package recovery

import (
	"fmt"
	"net/http"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// walletHandler serves the fullnode endpoint a recovering node calls to rebuild
// its wallet. It streams the DID's recovery data in two size-limited phases: the
// tokens phase sends every token the DID owns (RBT/FT/NFT, any status) with its
// chain structure but no blobs, then the tx phase sends the referenced
// transaction blobs, each one once. The ownership challenge gates both phases;
// the single-use nonce is retired only when the tx phase completes.
func (svc *Service) walletHandler(req *ensweb.Request) *ensweb.Result {
	if httpReq := req.GetHTTPRequest(); httpReq != nil && httpReq.Body != nil {
		httpReq.Body = http.MaxBytesReader(req.GetHTTPWritter(), httpReq.Body, recoverMaxRequestBodyBytes)
	}

	var recReq RecoverFromFullnodeRequest
	if err := svc.l.ParseJSON(req, &recReq); err != nil {
		svc.log.Warn("walletHandler: parse request body failed", "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if recReq.DID == "" {
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "did is required"}, http.StatusOK)
	}

	// Ownership gate: only the holder of the DID's private key may pull its
	// chain. Verify the signed challenge before serving any chain data. This stops
	// anyone who only knows the public DID from reading its token chains
	// (including smart-contract payloads) or loading the fullnode with
	// unauthenticated paginated pulls.
	var ownerNonce, ownerSig string
	if httpReq := req.GetHTTPRequest(); httpReq != nil {
		ownerNonce = httpReq.Header.Get(headerRecoveryNonce)
		ownerSig = httpReq.Header.Get(headerRecoverySignature)
	}
	if err := svc.verifyOwnership(recReq.DID, ownerNonce, ownerSig); err != nil {
		svc.log.Warn("walletHandler: ownership verification failed", "did", recReq.DID, "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "recovery authorization failed"}, http.StatusOK)
	}

	// Dispatch by phase. An empty phase is the start of a fresh recovery.
	phase := recReq.Cursor.Phase
	if phase == "" {
		phase = PhaseTokens
	}
	switch phase {
	case PhaseTokens:
		return svc.buildTokenPage(req, &recReq, ownerNonce)
	case PhaseTx:
		return svc.buildTxPage(req, &recReq, ownerNonce)
	default:
		svc.log.Warn("walletHandler: unknown recovery phase", "did", recReq.DID, "phase", phase)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "unknown recovery phase"}, http.StatusOK)
	}
}

// buildTokenPage emits one size-limited page of the token list. The tokens phase
// is never the end of recovery (the tx phase always follows), so HasMore is
// always true here; NextCursor either continues the token list or advances to
// the tx phase once it is exhausted.
func (svc *Service) buildTokenPage(req *ensweb.Request, recReq *RecoverFromFullnodeRequest, ownerNonce string) *ensweb.Result {
	cursorTokenID := recReq.Cursor.LastTokenID
	cursorPosition := recReq.Cursor.LastPosition
	if cursorTokenID == "" {
		// Empty cursor = start fresh; -1 makes (token_id, position) > ('', -1)
		// match the very first row of the first token.
		cursorPosition = -1
	}

	rows, err := svc.store.GetOwnedTokenPage(svc.ctx, recReq.DID, cursorTokenID, cursorPosition, recoverBatchSize)
	if err != nil {
		svc.log.Warn("buildTokenPage: token fetch failed",
			"did", recReq.DID, "cursor", fmt.Sprintf("(%s,%d)", cursorTokenID, cursorPosition), "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}

	tokensByID := make(map[string]*RecoveredToken)
	ordered := make([]string, 0)
	buildResult := func() RecoverFromFullnodeResult {
		out := make([]RecoveredToken, 0, len(ordered))
		for _, tid := range ordered {
			out = append(out, *tokensByID[tid])
		}
		return RecoverFromFullnodeResult{Phase: PhaseTokens, Tokens: out}
	}

	rowsIncluded := 0
	lastCompressed := 0
	pageFull := false
	var nextTokenID string
	var nextPosition int64

	for i := range rows {
		r := &rows[i]

		acc, existed := tokensByID[r.TokenID]
		wasNew := !existed
		if !existed {
			parent := ""
			if r.ParentTokenID != nil {
				parent = *r.ParentTokenID
			}
			acc = &RecoveredToken{
				TokenID:   r.TokenID,
				TokenType: r.TokenType,
				CurrentState: RecoveredTokenState{
					DID:            r.DID,
					TokenStatus:    r.TokenStatus,
					TokenValue:     r.TokenValue,
					TokenStateHash: r.TokenStateHash,
					TransactionID:  r.TransactionID,
					LatestPosition: r.LatestPosition,
					LatestRole:     r.LatestRole,
					ParentTokenID:  parent,
				},
			}
			tokensByID[r.TokenID] = acc
			ordered = append(ordered, r.TokenID)
		}
		prevTx := ""
		if r.PrevTxID != nil {
			prevTx = *r.PrevTxID
		}
		acc.Chain = append(acc.Chain, ChainRef{
			TxID:     r.ChainTxID,
			Position: r.Position,
			PrevTxID: prevTx,
			Role:     r.Role,
		})

		cz, fits := pageFits(&models.BasicResponse{Status: true, Message: "ok", Result: buildResult()}, rowsIncluded)
		if !fits {
			// Roll back the tentative addition and stop the page.
			acc.Chain = acc.Chain[:len(acc.Chain)-1]
			if wasNew {
				delete(tokensByID, r.TokenID)
				ordered = ordered[:len(ordered)-1]
			}
			pageFull = true
			break
		}
		rowsIncluded++
		lastCompressed = cz
		nextTokenID = r.TokenID
		nextPosition = r.Position
	}

	result := buildResult()
	result.HasMore = true // tx phase always follows the token list
	if pageFull || len(rows) == recoverBatchSize {
		// More token rows remain, stay in the tokens phase.
		result.NextCursor = RecoveryCursor{
			Phase:        PhaseTokens,
			LastTokenID:  nextTokenID,
			LastPosition: nextPosition,
		}
	} else {
		// Token list done, advance to the transaction phase.
		result.NextCursor = RecoveryCursor{Phase: PhaseTx}
	}

	svc.log.Info("buildTokenPage: returning",
		"did", recReq.DID,
		"rows_included", rowsIncluded,
		"tokens_in_page", len(result.Tokens),
		"compressed_bytes", lastCompressed,
		"next_phase", result.NextCursor.Phase,
		"next_cursor", fmt.Sprintf("(%s,%d)", result.NextCursor.LastTokenID, result.NextCursor.LastPosition))
	return renderGzipFixedLengthJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}

// buildTxPage emits one size-limited page of the transaction stream, each
// transaction once. When the stream is exhausted (HasMore=false) the recovery is
// complete and the single-use nonce is retired.
func (svc *Service) buildTxPage(req *ensweb.Request, recReq *RecoverFromFullnodeRequest, ownerNonce string) *ensweb.Result {
	cursorTxID := recReq.Cursor.LastTxID

	txns, err := svc.store.GetTransactionPage(svc.ctx, recReq.DID, cursorTxID, recoverBatchSize)
	if err != nil {
		svc.log.Warn("buildTxPage: transaction fetch failed",
			"did", recReq.DID, "cursor", cursorTxID, "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "fullnode read failed"}, http.StatusOK)
	}

	included := make([]RecoveredTransaction, 0, len(txns))
	buildResult := func() RecoverFromFullnodeResult {
		return RecoverFromFullnodeResult{Phase: PhaseTx, Transactions: included}
	}

	rowsIncluded := 0
	lastCompressed := 0
	pageFull := false
	var lastTxID string

	for i := range txns {
		included = append(included, txns[i])
		cz, fits := pageFits(&models.BasicResponse{Status: true, Message: "ok", Result: buildResult()}, rowsIncluded)
		if !fits {
			included = included[:len(included)-1]
			pageFull = true
			break
		}
		rowsIncluded++
		lastCompressed = cz
		lastTxID = txns[i].ID
	}

	hasMore := pageFull || len(txns) == recoverBatchSize
	result := buildResult()
	result.HasMore = hasMore
	if hasMore {
		result.NextCursor = RecoveryCursor{Phase: PhaseTx, LastTxID: lastTxID}
	} else {
		// Recovery is complete, retire the single-use nonce so it can't be
		// replayed. While pages remain the nonce stays valid.
		svc.sessions.remove(ownerNonce)
	}

	svc.log.Info("buildTxPage: returning",
		"did", recReq.DID,
		"txns_included", rowsIncluded,
		"compressed_bytes", lastCompressed,
		"has_more", hasMore,
		"next_tx_cursor", result.NextCursor.LastTxID)
	return renderGzipFixedLengthJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: result}, http.StatusOK)
}
