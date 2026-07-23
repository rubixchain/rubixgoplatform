package recovery

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

const (
	// A recovery nonce stays valid for the whole run but is reaped once idle.
	// Every page refreshes the session, and pages arrive well within this window,
	// so an active recovery (even a large multi-page wallet) never expires; an
	// abandoned or spammed session is swept. Idle, not absolute, on purpose.
	recoverySessionIdleTTL = 15 * time.Minute

	// Hard cap on live recovery sessions. challengeHandler mints a session for any
	// DID without authentication (the nonce is the thing that later proves
	// ownership), so this bounds the map against unauthenticated challenge spam.
	// Far above the handful of concurrent recoveries a fullnode actually serves.
	recoverySessionMaxLive = 1024
)

// Ownership proof travels in HTTP headers (not the request body) so it can be
// checked before the body is parsed. Same convention as the X-API-Key /
// Authorization headers.
const (
	headerRecoveryNonce     = "X-Rubix-Recovery-Nonce"
	headerRecoverySignature = "X-Rubix-Recovery-Signature"
)

// recoveryNonceHash builds the digest the recovering node signs to prove DID
// ownership: SHA3-256 over the fullnode-issued nonce. Client and fullnode build
// it the same way, using the SHA3-256 the rest of the DID-signing flow uses.
func recoveryNonceHash(nonce string) []byte {
	return util.CalculateHash([]byte("recover-from-fullnode:"+nonce), constants.HashAlgorithm_SHA3_256)
}

// newRecoveryNonce mints a 256-bit random, single-use nonce.
func newRecoveryNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return util.BytesToBase64(b), nil
}

// session is one in-flight, ownership-proven recovery, bound to a DID. validated
// flips true after the first signature check so later pages only pay a cheap
// liveness lookup. lastSeen is refreshed on every page and drives idle eviction.
type session struct {
	did       string
	validated bool
	lastSeen  time.Time
}

// sessionStore holds the single-use nonces issued to recovering nodes. A nonce
// stays valid for the whole recovery (refreshed per page) and is removed on
// completion so a captured nonce cannot be replayed afterwards. Idle sessions
// are swept after recoverySessionIdleTTL and the map size is capped so
// unauthenticated challenge minting cannot grow it without bound.
type sessionStore struct {
	mu sync.Mutex
	m  map[string]*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{m: make(map[string]*session)}
}

// create records a new session, first reaping idle ones. It rejects the mint
// once the live-session cap is hit so a spammed fullnode fails fast instead of
// growing the map without bound.
func (s *sessionStore) create(nonce, did string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if len(s.m) >= recoverySessionMaxLive {
		return fmt.Errorf("too many active recovery sessions")
	}
	s.m[nonce] = &session{did: did, lastSeen: time.Now()}
	return nil
}

// lookup returns the session's DID and validated flag for a live nonce and
// refreshes its lastSeen (sliding idle window). An idle-expired nonce is evicted
// and reported as absent.
func (s *sessionStore) lookup(nonce string) (did string, validated, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, exists := s.m[nonce]
	if !exists {
		return "", false, false
	}
	if time.Since(sess.lastSeen) > recoverySessionIdleTTL {
		delete(s.m, nonce)
		return "", false, false
	}
	sess.lastSeen = time.Now()
	return sess.did, sess.validated, true
}

// sweepLocked drops idle sessions. Caller holds s.mu.
func (s *sessionStore) sweepLocked() {
	for nonce, sess := range s.m {
		if time.Since(sess.lastSeen) > recoverySessionIdleTTL {
			delete(s.m, nonce)
		}
	}
}

func (s *sessionStore) markValidated(nonce string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.m[nonce]; ok {
		sess.validated = true
	}
}

func (s *sessionStore) remove(nonce string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, nonce)
}

// verifyOwnership confirms the requester holds the private key for reqDID before
// any chain data is served. The nonce must belong to a live session for this
// DID. On the first page the signature is checked against the DID's public key
// over the nonce digest and the session is marked validated, so later pages only
// do the liveness lookup. The nonce is valid for the whole recovery and evicted
// on completion. nonce and signature come from the X-Rubix-Recovery-* headers.
func (svc *Service) verifyOwnership(reqDID, nonce, signature string) error {
	if nonce == "" || signature == "" {
		return fmt.Errorf("missing recovery nonce/signature header")
	}
	did, validated, ok := svc.sessions.lookup(nonce)
	if !ok {
		return fmt.Errorf("unknown or completed recovery nonce")
	}
	if did != reqDID {
		return fmt.Errorf("nonce does not belong to this DID")
	}
	if validated {
		return nil
	}
	sigBytes, err := util.Base64ToBytes(signature)
	if err != nil {
		return fmt.Errorf("decode ownership signature: %w", err)
	}
	verified, err := svc.verifyOwner(reqDID, recoveryNonceHash(nonce), sigBytes)
	if err != nil {
		return fmt.Errorf("verify ownership signature: %w", err)
	}
	if !verified {
		return fmt.Errorf("ownership signature does not match DID public key")
	}
	svc.sessions.markValidated(nonce)
	return nil
}

// challengeHandler mints a one-time nonce bound to the requested DID and returns
// it. The caller signs the nonce and presents nonce + signature on each page.
func (svc *Service) challengeHandler(req *ensweb.Request) *ensweb.Result {
	var chReq RecoverChallengeRequest
	if err := svc.l.ParseJSON(req, &chReq); err != nil {
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "Invalid input"}, http.StatusOK)
	}
	if chReq.DID == "" {
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "did is required"}, http.StatusOK)
	}
	nonce, err := newRecoveryNonce()
	if err != nil {
		svc.log.Warn("challengeHandler: nonce generation failed", "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "failed to mint nonce"}, http.StatusOK)
	}
	if err := svc.sessions.create(nonce, chReq.DID); err != nil {
		svc.log.Warn("challengeHandler: session store full", "did", chReq.DID, "err", err)
		return svc.l.RenderJSON(req, &models.BasicResponse{Status: false, Message: "recovery temporarily unavailable, retry shortly"}, http.StatusOK)
	}
	svc.log.Info("challengeHandler: issued nonce", "did", chReq.DID)
	return svc.l.RenderJSON(req, &models.BasicResponse{Status: true, Message: "ok", Result: RecoverChallengeResult{Nonce: nonce}}, http.StatusOK)
}
