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
		splitter: txnWithInitiator(t, "whole-genesis", minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, true, false); err != nil {
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
		sender: txnWithInitiator(t, "whole-genesis", minter), // splitter unreachable
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, true, false); err != nil {
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
		sender: txnWithInitiator(t, "whole-genesis", minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, true, false); err != nil {
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
		splitter: txnWithInitiator(t, "whole-genesis", minter),
	}}

	if err := validateMinterAllowlist(partTransfer(splitter), false, w, testLogger(), f.fetch, true, false); err != nil {
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

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, true, false)
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

	if err := validateMinterAllowlist(partTransfer(validDID('n')), false, w, testLogger(), f.fetch, true, false); err != nil {
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
		splitter: txnWithInitiator(t, "whole-genesis", rogue),
	}}

	err := validateMinterAllowlist(partTransfer(sender), false, w, testLogger(), f.fetch, true, false)
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
		splitter: txnWithInitiator(t, "whole-genesis", minter),
	}}

	if err := validateMinterAllowlist(partTransfer(sender), true, w, testLogger(), f.fetch, true, false); err != nil {
		t.Fatalf("expected the fullnode chain to yield the splitter, got: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != splitter {
		t.Errorf("expected one fetch from the splitter %s; got %v", splitter, f.calls)
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

	if err := validateMinterAllowlist(txnInfo, false, w, testLogger(), f.fetch, true, false); err != nil {
		t.Fatalf("unexpected error for a whole-token mint: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("a whole-token mint must not trigger a peer fetch; got %v", f.calls)
	}
}
