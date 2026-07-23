package recovery

import (
	"context"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/setup"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// OwnerVerifier reports whether `did` produced `sig` over `digest`, resolving the
// DID's public key. Core supplies this (InitialiseDID + SignVerify) so the
// recovery package stays free of DID-crypto internals.
type OwnerVerifier func(did string, digest, sig []byte) (bool, error)

// Service is the fullnode-side recovery endpoint handler. It owns the ownership
// nonce sessions and serves recovery pages from the fullnode state tables.
type Service struct {
	l           *ipfsport.Listener
	store       *Store
	sessions    *sessionStore
	verifyOwner OwnerVerifier
	ctx         context.Context
	log         logger.Logger
}

// New builds a recovery Service. verifyOwner resolves and checks DID signatures.
func New(l *ipfsport.Listener, w *wallet.Wallet, verifyOwner OwnerVerifier, log logger.Logger) *Service {
	rlog := log.Named("recovery")
	return &Service{
		l:           l,
		store:       NewStore(w, rlog),
		sessions:    newSessionStore(),
		verifyOwner: verifyOwner,
		ctx:         w.Ctx,
		log:         rlog,
	}
}

// RegisterRoutes wires the fullnode-served recovery endpoints onto the listener.
func (svc *Service) RegisterRoutes() {
	svc.l.AddRoute(setup.APIRecoverWalletChallenge, "POST", svc.challengeHandler)
	svc.l.AddRoute(setup.APIRecoverWallet, "POST", svc.walletHandler)
}
