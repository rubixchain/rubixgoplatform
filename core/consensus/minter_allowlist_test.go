package consensus

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// -----------------------------------------------------------------------------
// Fixtures
//
// The scenario mirrors the reported incident: part token 50001_1000949_3 whose
// whole-token ancestor 50001_1000949 was minted by the testnet faucet DID, split
// by one node, and then transferred on by a later holder that never received the
// (burnt) whole token's chain.
// -----------------------------------------------------------------------------

const (
	partTokenID  = "50001_1000949_3"
	wholeTokenID = "50001_1000949"
)

// allowedTestnetMinter is TestnetAllowedMinters[0], which covers level 50001
// token numbers 1..2000000 — so 1000949 is inside its range.
func allowedTestnetMinter(t *testing.T) string {
	t.Helper()
	if len(minterallowlist.TestnetAllowedMinters) == 0 {
		t.Fatal("TestnetAllowedMinters is empty; test fixture needs updating")
	}
	m := minterallowlist.TestnetAllowedMinters[0]
	if m.Level != 50001 || 1000949 < m.StartTokenNumber || 1000949 > m.EndTokenNumber {
		t.Fatalf("fixture drift: token 50001_1000949 no longer falls in %s range %d..%d",
			m.DID, m.StartTokenNumber, m.EndTokenNumber)
	}
	return m.DID
}

// fakeGenesisLookup is an in-memory genesisInitiatorLookup. Absent keys return
// errors, standing in for "this node does not hold that chain".
type fakeGenesisLookup struct {
	genesisInitiator   map[string]string               // wholeID -> minter DID
	heightZero         map[string]*models.Transactions // tokenID -> position-0 txn
	heightZeroFullnode map[string]*models.Transactions // same, fullnode_tokenchain
}

func (f *fakeGenesisLookup) GetGenesisInitiatorDID(tokenID string, isFullNode bool) (string, error) {
	if did, ok := f.genesisInitiator[tokenID]; ok {
		return did, nil
	}
	return "", fmt.Errorf("GetGenesisInitiatorDID: no genesis for token %s", tokenID)
}

func (f *fakeGenesisLookup) GetTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	if tx, ok := f.heightZero[tokenID]; ok {
		return tx, 1, nil
	}
	return nil, -1, fmt.Errorf("transaction not found at height %d for token %s", height, tokenID)
}

func (f *fakeGenesisLookup) GetFullNodeTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	if tx, ok := f.heightZeroFullnode[tokenID]; ok {
		return tx, 1, nil
	}
	return nil, -1, fmt.Errorf("fullnode transaction not found at height %d for token %s", height, tokenID)
}

// fetchRecorder fakes the genesis-only peer fetch and records which peers were
// asked, in order — the ordering is the behaviour under test.
type fetchRecorder struct {
	responses map[string]*models.Transactions // peerDID -> genesis it will serve
	calls     []string
}

func (f *fetchRecorder) fetch(peerDID, tokenID string) (*models.Transactions, error) {
	f.calls = append(f.calls, peerDID)
	if tx, ok := f.responses[peerDID]; ok {
		return tx, nil
	}
	return nil, fmt.Errorf("peer returned error: transaction not found at height 0 for token %s", tokenID)
}

// syncRecorder fakes Core.SyncBurntTokenChainFromPeer, recording which
// (peer, token) pairs the gate asked to persist.
type syncRecorder struct {
	err   error // forced failure
	calls [][2]string
}

func (s *syncRecorder) sync(peerDID, tokenID string) error {
	s.calls = append(s.calls, [2]string{peerDID, tokenID})
	return s.err
}

// txnWithInitiator builds a stored transaction whose Info carries only the
// initiator — the single field both resolveSplitInitiator and the minter
// resolution read.
func txnWithInitiator(t *testing.T, id, initiator string) *models.Transactions {
	t.Helper()
	info, err := json.Marshal(models.TransactionInfo{Initiator: initiator})
	if err != nil {
		t.Fatalf("marshal transaction info: %v", err)
	}
	return &models.Transactions{ID: id, Info: info}
}

// wholeGenesisTxn builds a whole-token genesis shaped the way
// Wallet.PersistGenesisTokenRecord writes one: Initiator == Owner == the minting
// DID, minting wholeTokenID with no predecessor. genesisMintsToken requires
// exactly this shape, so a peer serving anything else is rejected.
func wholeGenesisTxn(t *testing.T, minter string) *models.Transactions {
	t.Helper()
	return wholeGenesisTxnFor(t, minter, wholeTokenID)
}

// wholeGenesisTxnFor is wholeGenesisTxn for an arbitrary token ID, so tests can
// build the mismatched genesis a hostile peer would replay.
func wholeGenesisTxnFor(t *testing.T, minter, tokenID string) *models.Transactions {
	t.Helper()
	info, err := json.Marshal(models.TransactionInfo{
		Initiator: minter,
		Owner:     minter,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{TokenID: tokenID, PreviousTransactionID: "", TokenValue: 1.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal genesis info: %v", err)
	}
	sig, err := json.Marshal(models.Signature{InitiatorSignature: "c2lnbmF0dXJl"})
	if err != nil {
		t.Fatalf("marshal genesis signature: %v", err)
	}
	return &models.Transactions{ID: "whole-genesis", Info: info, Signature: sig}
}

// sigRecorder fakes Core.verifyGenesisSignature, recording what it was asked to
// verify so tests can assert the claimed minter is the DID being checked.
type sigRecorder struct {
	err   error // forced failure
	calls [][2]string
}

func (s *sigRecorder) verify(signerDID string, _ *models.TransactionInfo, signature string) error {
	s.calls = append(s.calls, [2]string{signerDID, signature})
	return s.err
}

// partTransfer is a transaction moving the part token on, initiated by a holder
// that is NOT the splitter — the second-hop case that used to fail.
func partTransfer(sender string) *models.TransactionInfo {
	return &models.TransactionInfo{
		Initiator: sender,
		Owner:     validDID('r'),
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{TokenID: partTokenID, PreviousTransactionID: "prev-tx"},
			},
		},
	}
}

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

// The splitter held the whole-token chain at split time, so it is asked first
// and the sender is never contacted.
func TestValidateMinterAllowlistPartTokenAsksSplitterFirst(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{}, // whole-token chain absent locally
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("expected the split initiator to resolve the minter, got: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != splitter {
		t.Errorf("expected exactly one fetch, from the splitter %s; got %v", splitter, f.calls)
	}
}

// When the splitter cannot serve the genesis, the previously-used peer is still
// tried — the new ordering adds a candidate, it does not remove one.
func TestValidateMinterAllowlistPartTokenFallsBackToDeclaredPeer(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		sender: wholeGenesisTxn(t, minter), // splitter unreachable
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("expected fallback to the declared peer to succeed, got: %v", err)
	}
	want := []string{splitter, sender}
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		t.Errorf("expected peers tried in order %v; got %v", want, f.calls)
	}
}

// If the part token's own genesis is not held locally the splitter is unknown,
// and behaviour falls back to exactly what it was before this change.
func TestValidateMinterAllowlistPartTokenNoLocalSplitGenesis(t *testing.T) {
	minter := allowedTestnetMinter(t)
	sender := validDID('n')

	w := &fakeGenesisLookup{genesisInitiator: map[string]string{}} // nothing local at all
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		sender: wholeGenesisTxn(t, minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("expected the declared peer to be used when the splitter is unknown, got: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != sender {
		t.Errorf("expected exactly one fetch, from the sender %s; got %v", sender, f.calls)
	}
}

// On the first hop after a split the splitter IS the sender; it must not be
// asked twice.
func TestValidateMinterAllowlistPartTokenSplitterIsSenderNoDuplicateFetch(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter := validDID('s')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}

	if err := validateMinterAllowlist(partTransfer(splitter), false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("expected the deduplicated peer list to produce one fetch; got %v", f.calls)
	}
}

// When no peer can serve the genesis the error must name every peer tried, so
// the failure is diagnosable from the message alone.
func TestValidateMinterAllowlistPartTokenAllPeersFail(t *testing.T) {
	allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{}} // nobody has it

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false)
	if err == nil {
		t.Fatal("expected an error when no peer can serve the whole-token genesis")
	}
	for _, want := range []string{"minter unverifiable", partTokenID, wholeTokenID, splitter, sender} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q; got: %v", want, err)
		}
	}
}

// A locally-held whole-token chain short-circuits everything: no network at all.
func TestValidateMinterAllowlistLocalGenesisSkipsFetch(t *testing.T) {
	minter := allowedTestnetMinter(t)

	w := &fakeGenesisLookup{genesisInitiator: map[string]string{wholeTokenID: minter}}
	f := &fetchRecorder{responses: map[string]*models.Transactions{}}

	if err := validateMinterAllowlist(partTransfer(validDID('n')), false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("expected no peer fetch when the genesis is local; got %v", f.calls)
	}
}

// Resolving the minter via the splitter must not weaken the gate: an
// unauthorised minter is still rejected.
func TestValidateMinterAllowlistUnauthorisedMinterRejected(t *testing.T) {
	splitter, sender := validDID('s'), validDID('n')
	rogue := validDID('x')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, rogue),
	}}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false)
	if err == nil {
		t.Fatal("expected rejection for a minter outside the allowlist")
	}
	if !strings.Contains(err.Error(), "not authorised") {
		t.Errorf("expected an authorisation rejection; got: %v", err)
	}
}

// A fullnode reads its own fullnode_tokenchain for the split genesis.
func TestValidateMinterAllowlistFullnodeReadsFullnodeChain(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		// Deliberately empty: a fullnode must not fall back to the wallet table.
		heightZero: map[string]*models.Transactions{},
		heightZeroFullnode: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), true, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("expected the fullnode chain to yield the splitter, got: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != splitter {
		t.Errorf("expected one fetch from the splitter %s; got %v", splitter, f.calls)
	}
}

// -----------------------------------------------------------------------------
// Persisting the burnt ancestor's chain
// -----------------------------------------------------------------------------

// After resolving the minter from a peer, the whole token's chain is pulled from
// that same peer — keyed by the WHOLE token id, so every sibling part benefits.
func TestValidateMinterAllowlistPersistsBurntAncestorChain(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}
	sr := &syncRecorder{}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, nil, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sr.calls) != 1 {
		t.Fatalf("expected one chain-persist call; got %v", sr.calls)
	}
	if sr.calls[0] != [2]string{splitter, wholeTokenID} {
		t.Errorf("expected the chain to be pulled from %s for %s; got %v", splitter, wholeTokenID, sr.calls[0])
	}
}

// The chain is requested from whichever peer actually answered, not blindly from
// the first candidate.
func TestValidateMinterAllowlistPersistsFromServingPeer(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		sender: wholeGenesisTxn(t, minter), // splitter unreachable
	}}
	sr := &syncRecorder{}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, nil, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sr.calls) != 1 || sr.calls[0][0] != sender {
		t.Errorf("expected the chain to be pulled from the peer that served the genesis (%s); got %v", sender, sr.calls)
	}
}

// Persistence is an optimisation only — a failure must not fail the transaction
// that has already been validated.
func TestValidateMinterAllowlistChainPersistFailureNonFatal(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}
	sr := &syncRecorder{err: fmt.Errorf("peer went away")}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, nil, true, false); err != nil {
		t.Fatalf("a failed chain persist must not fail validation, got: %v", err)
	}
}

// Nothing is persisted when no peer could serve the genesis — there is no
// verified chain to take.
func TestValidateMinterAllowlistNoPersistWhenUnresolved(t *testing.T) {
	allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{}}
	sr := &syncRecorder{}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, nil, true, false); err == nil {
		t.Fatal("expected an error when no peer can serve the genesis")
	}
	if len(sr.calls) != 0 {
		t.Errorf("nothing should be persisted when resolution failed; got %v", sr.calls)
	}
}

// A locally-resolvable minter means no fetch and nothing to persist.
func TestValidateMinterAllowlistNoPersistWhenLocal(t *testing.T) {
	minter := allowedTestnetMinter(t)

	w := &fakeGenesisLookup{genesisInitiator: map[string]string{wholeTokenID: minter}}
	f := &fetchRecorder{responses: map[string]*models.Transactions{}}
	sr := &syncRecorder{}

	if err := validateMinterAllowlist(partTransfer(validDID('n')), false, w, testLogger(), f.fetch, sr.sync, nil, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sr.calls) != 0 {
		t.Errorf("a local hit must not trigger a chain pull; got %v", sr.calls)
	}
}

// Whole-token minting is untouched by this change: the transaction that IS the
// genesis still resolves its minter from its own initiator, with no peer fetch.
func TestValidateMinterAllowlistWholeTokenGenesisUnchanged(t *testing.T) {
	minter := allowedTestnetMinter(t)

	txnInfo := &models.TransactionInfo{
		Initiator: minter,
		Owner:     validDID('r'),
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{{TokenID: wholeTokenID, PreviousTransactionID: ""}},
		},
	}
	w := &fakeGenesisLookup{genesisInitiator: map[string]string{}}
	f := &fetchRecorder{responses: map[string]*models.Transactions{}}

	if err := validateMinterAllowlist(txnInfo, false, w, testLogger(), f.fetch, nil, nil, true, false); err != nil {
		t.Fatalf("unexpected error for a whole-token mint: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("a whole-token mint must not trigger a peer fetch; got %v", f.calls)
	}
}

// -----------------------------------------------------------------------------
// Guard 1 — the served genesis must be the genesis of the token we asked about.
// -----------------------------------------------------------------------------

// The attack this closes: a hostile peer replays a genuine genesis belonging to
// some OTHER token that an allowlisted DID really did mint. Reading only
// Initiator, the gate would accept it, making one legitimate mint a skeleton key
// for every part token on the network.
func TestValidateMinterAllowlistRejectsGenesisForDifferentToken(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	// Genuine genesis, genuinely allowlisted minter — but for a different token.
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxnFor(t, minter, "50001_7777777"),
		sender:   wholeGenesisTxnFor(t, minter, "50001_7777777"),
	}}
	sr := &syncRecorder{}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, nil, true, false)
	if err == nil {
		t.Fatal("expected a replayed genesis for another token to be rejected")
	}
	if !strings.Contains(err.Error(), "does not mint") {
		t.Fatalf("expected a binding failure, got: %v", err)
	}
	// Nothing forged may be cached, or every later validation reads it back from
	// the local chain and trusts it without re-fetching.
	if len(sr.calls) != 0 {
		t.Errorf("a rejected genesis must never be persisted; got %v", sr.calls)
	}
}

// A genesis carrying no token list at all cannot bind to anything.
func TestValidateMinterAllowlistRejectsGenesisWithoutMintedToken(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: txnWithInitiator(t, "whole-genesis", minter), // Info has Initiator only
		sender:   txnWithInitiator(t, "whole-genesis", minter),
	}}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false)
	if err == nil || !strings.Contains(err.Error(), "does not mint") {
		t.Fatalf("expected a genesis with no minted token to be rejected, got: %v", err)
	}
}

// A part token's own genesis must not satisfy the whole token's. The part chain
// is authored and signed by the splitter, so accepting it would defeat the
// entire point of resolving the whole-token ancestor.
func TestValidateMinterAllowlistRejectsPartGenesisAsWholeGenesis(t *testing.T) {
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxnFor(t, splitter, partTokenID),
		sender:   wholeGenesisTxnFor(t, splitter, partTokenID),
	}}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, nil, true, false)
	if err == nil || !strings.Contains(err.Error(), "does not mint") {
		t.Fatalf("expected the part-token genesis to be rejected as the whole's, got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Guard 2 — the claimed minter must have signed the genesis.
// -----------------------------------------------------------------------------

// The DID handed to the verifier must be the one the genesis names as minter —
// the same DID the allowlist is about to authorise.
func TestValidateMinterAllowlistVerifiesGenesisAgainstClaimedMinter(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}
	vr := &sigRecorder{}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, vr.verify, true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vr.calls) != 1 {
		t.Fatalf("expected exactly one signature verification, got %d", len(vr.calls))
	}
	if vr.calls[0][0] != minter {
		t.Errorf("expected the claimed minter %s to be verified, got %s", minter, vr.calls[0][0])
	}
	if vr.calls[0][1] != "c2lnbmF0dXJl" {
		t.Errorf("expected the genesis initiator signature to be passed through, got %q", vr.calls[0][1])
	}
}

// Staged rollout: while enforceGenesisSignature is false a verification failure
// is recorded but must not reject, so resolving a minter DID over the network
// cannot take down mainnet transfers. Flipping the constant makes this reject;
// this test pins the current, deliberate behaviour.
func TestValidateMinterAllowlistSignatureFailureIsLogOnlyWhileStaged(t *testing.T) {
	if enforceGenesisSignature {
		t.Skip("enforceGenesisSignature is on; signature failures now reject by design")
	}
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}
	vr := &sigRecorder{err: fmt.Errorf("could not resolve minter DID document")}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, vr.verify, true, false); err != nil {
		t.Fatalf("while staged, a signature failure must not reject; got: %v", err)
	}
	if len(vr.calls) != 1 {
		t.Errorf("expected the verification to still be attempted, got %d calls", len(vr.calls))
	}
}

// Guard 1 runs before Guard 2, so a genesis that fails binding is rejected
// without spending a network round trip resolving a DID document.
func TestValidateMinterAllowlistSkipsSignatureCheckWhenBindingFails(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxnFor(t, minter, "50001_7777777"),
		sender:   wholeGenesisTxnFor(t, minter, "50001_7777777"),
	}}
	vr := &sigRecorder{}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, vr.verify, true, false); err == nil {
		t.Fatal("expected rejection on the binding check")
	}
	if len(vr.calls) != 0 {
		t.Errorf("binding must be checked first; got %d signature verifications", len(vr.calls))
	}
}

// A genesis with no signature at all is a verification failure, not a panic.
func TestValidateMinterAllowlistHandlesGenesisWithoutSignature(t *testing.T) {
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	unsigned := wholeGenesisTxn(t, minter)
	unsigned.Signature = nil

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{splitter: unsigned}}
	vr := &sigRecorder{}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, nil, vr.verify, true, false)
	if enforceGenesisSignature {
		if err == nil || !strings.Contains(err.Error(), "carries no signature") {
			t.Fatalf("expected an unsigned genesis to be rejected, got: %v", err)
		}
	} else if err != nil {
		t.Fatalf("while staged, a missing signature must not reject; got: %v", err)
	}
	// Either way the verifier is never reached: there is nothing to verify.
	if len(vr.calls) != 0 {
		t.Errorf("an absent signature has nothing to verify; got %v", vr.calls)
	}
}

// The enforcing counterpart of the staged test above: once
// enforceGenesisSignature is on, a genesis whose signature does not verify is
// rejected outright rather than merely logged.
func TestValidateMinterAllowlistRejectsUnverifiedSignatureWhenEnforcing(t *testing.T) {
	if !enforceGenesisSignature {
		t.Skip("enforceGenesisSignature is off; failures are log-only by design")
	}
	minter := allowedTestnetMinter(t)
	splitter, sender := validDID('s'), validDID('n')

	w := &fakeGenesisLookup{
		genesisInitiator: map[string]string{},
		heightZero: map[string]*models.Transactions{
			partTokenID: txnWithInitiator(t, "split-tx", splitter),
		},
	}
	f := &fetchRecorder{responses: map[string]*models.Transactions{
		splitter: wholeGenesisTxn(t, minter),
	}}
	sr := &syncRecorder{}
	vr := &sigRecorder{err: fmt.Errorf("signature verification returned false")}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, sr.sync, vr.verify, true, false)
	if err == nil || !strings.Contains(err.Error(), "genesis signature invalid") {
		t.Fatalf("expected an unverified genesis to be rejected, got: %v", err)
	}
	// A genesis that failed verification must not be cached either.
	if len(sr.calls) != 0 {
		t.Errorf("an unverified genesis must never be persisted; got %v", sr.calls)
	}
}
