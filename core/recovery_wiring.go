package core

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/recovery"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// registerRecoveryRoutes builds the recovery service and registers its
// fullnode-served endpoints. Called from SubscribeTxnSetup on a fullnode.
func (c *Core) registerRecoveryRoutes() {
	if c.recoverySvc == nil {
		c.recoverySvc = recovery.New(c.l, c.w, c.verifyRecoveryOwner, c.log)
	}
	c.recoverySvc.RegisterRoutes()
}

// verifyRecoveryOwner resolves the DID's public key and checks the signature. It
// is injected into the recovery service so that package stays free of DID-crypto
// internals.
func (c *Core) verifyRecoveryOwner(did string, digest, sig []byte) (bool, error) {
	dc, err := c.InitialiseDID(did)
	if err != nil {
		return false, err
	}
	return dc.SignVerify(digest, sig)
}

// RecoverWalletFromFullnodeAsync runs wallet recovery in the background and
// delivers the summary over the request's signature channel. Recovery signs an
// ownership challenge, so it runs through the same OutChan/InChan signature flow
// as register/transfer: the node emits "Signature needed", the caller answers
// via /rubix/v1/signature, and the summary is returned on the same channel. opts
// carries the mode and filters; when allDIDs is set every local DID is recovered.
func (c *Core) RecoverWalletFromFullnodeAsync(reqID, did string, opts recovery.RecoverOptions, allDIDs bool) {
	if allDIDs {
		c.recoverAllWallets(reqID, opts)
		return
	}
	deps, err := c.buildRecoveryDeps(reqID, did)
	var result *recovery.WalletSyncResult
	if err == nil {
		result, err = recovery.RecoverWallet(c.w.Ctx, deps, did, opts)
	}
	c.deliverRecoveryResult(reqID, did, recoveryResultResponse(result, err))
}

// recoverAllWallets recovers every local DID on this node in turn with the given
// options and delivers an aggregate summary. A node with an external signer
// answers one signature challenge per DID.
func (c *Core) recoverAllWallets(reqID string, opts recovery.RecoverOptions) {
	dids := c.localRecoverableDIDs()
	if len(dids) == 0 {
		c.deliverRecoveryResult(reqID, "", &models.BasicResponse{Status: true, Message: "no local DIDs to recover"})
		return
	}
	var recovered, failed int
	parts := make([]string, 0, len(dids))
	for _, did := range dids {
		deps, err := c.buildRecoveryDeps(reqID, did)
		var result *recovery.WalletSyncResult
		if err == nil {
			result, err = recovery.RecoverWallet(c.w.Ctx, deps, did, opts)
		}
		if err != nil {
			failed++
			parts = append(parts, fmt.Sprintf("%s: failed: %v", did, err))
			continue
		}
		recovered++
		parts = append(parts, fmt.Sprintf("%s: %d tokens, %d pinned", did, result.TokensSeen, result.TokensPinned))
	}
	c.deliverRecoveryResult(reqID, "", &models.BasicResponse{
		Status:  failed == 0,
		Message: fmt.Sprintf("recovered %d/%d local DIDs; %s", recovered, len(dids), strings.Join(parts, "; ")),
	})
}

// buildRecoveryDeps assembles the client dependencies for a recovery run and
// sets up the signer for did. SetupDID may block on the operator signature via
// the web channel; the signer is invoked once per recovery inside RecoverWallet.
func (c *Core) buildRecoveryDeps(reqID, did string) (recovery.ClientDeps, error) {
	if did == "" {
		return recovery.ClientDeps{}, fmt.Errorf("buildRecoveryDeps: did is required")
	}
	signer, err := c.SetupDID(reqID, did)
	if err != nil {
		return recovery.ClientDeps{}, fmt.Errorf("buildRecoveryDeps: setup signer for %s: %w", did, err)
	}
	return recovery.ClientDeps{
		PM:       c.pm,
		Store:    recovery.NewStore(c.w, c.log),
		Wallet:   c.w,
		Signer:   signer,
		AppName:  c.getCoreAppName,
		Testnet:  c.testnet,
		Localnet: c.localnet,
		Log:      c.log,
	}, nil
}

// localRecoverableDIDs returns the DIDs this node holds keys for (local=true),
// the set eligible for whole-node recovery.
func (c *Core) localRecoverableDIDs() []string {
	all := c.GetDIDs()
	out := make([]string, 0, len(all))
	for _, did := range all {
		local, err := c.w.IsLocalDID(did)
		if err != nil {
			c.log.Warn("localRecoverableDIDs: IsLocalDID failed; skipping", "did", did, "err", err)
			continue
		}
		if local {
			out = append(out, did)
		}
	}
	return out
}

// recoveryResultResponse builds the operator response from a recovery summary.
// It leaves Result nil so the CLI signature loop does not mistake a success for
// another signature request.
func recoveryResultResponse(result *recovery.WalletSyncResult, err error) *models.BasicResponse {
	br := &models.BasicResponse{Status: true, Message: "wallet recovery completed"}
	if result != nil {
		br.Message = fmt.Sprintf("wallet recovery completed (%s): %d tokens, %d transactions, %d chain entries, %d pinned, %d/%d self-test",
			result.Mode, result.TokensSeen, result.TransactionsPersisted, result.ChainEntriesPersisted,
			result.TokensPinned, result.SelfTestOK, result.SelfTestOK+result.SelfTestFailed)
	}
	if err != nil {
		br.Status = false
		br.Message = "recovery failed: " + err.Error()
	}
	return br
}

// deliverRecoveryResult pushes the final recovery response onto the request's
// signature channel.
func (c *Core) deliverRecoveryResult(reqID, did string, br *models.BasicResponse) {
	dc := c.GetWebReq(reqID)
	if dc == nil {
		c.log.Error("deliverRecoveryResult: failed to get did channel", "did", did)
		return
	}
	dc.OutChan <- br
}
