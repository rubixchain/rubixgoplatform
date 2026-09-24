package consensus

import (
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

// fakeGenesisLookup stands in for the wallet, returning a fixed minter for any
// token.
type fakeGenesisLookup struct {
	minter string
}

func (f fakeGenesisLookup) GetGenesisInitiatorDID(tokenID string, isFullNode bool) (string, error) {
	return f.minter, nil
}

// txnWithToken builds the smallest TransactionInfo the allowlist check reads.
func txnWithToken(tokenID string) *models.TransactionInfo {
	return &models.TransactionInfo{
		Initiator: validDID('a'),
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{TokenID: tokenID, PreviousTransactionID: "sometxn"},
			},
		},
	}
}

const (
	customMinter = "bafybmicustomminter"
	otherMinter  = "bafybmisomeoneelse"
	// customLevel is level 1 of the custom network series.
	customLevel = constants.CustomNetRBT_Level_Offset + 1
)

// withCustomMinters points CustomNetAllowedMinters at a test list and restores
// the real one when the test ends.
func withCustomMinters(t *testing.T, minters ...minterallowlist.MintAccessRange) {
	t.Helper()
	saved := minterallowlist.CustomNetAllowedMinters
	minterallowlist.CustomNetAllowedMinters = minters
	t.Cleanup(func() { minterallowlist.CustomNetAllowedMinters = saved })
}

// TestCustomNetworkUsesItsOwnMinters covers the case the gate exists for: a
// testnet node on a non-Rubix swarm validates against CustomNetAllowedMinters.
func TestCustomNetworkUsesItsOwnMinters(t *testing.T) {
	withCustomMinters(t, minterallowlist.MintAccessRange{
		DID: customMinter, Level: customLevel, StartTokenNumber: 1, EndTokenNumber: 1000,
	})

	t.Run("listed minter passes", func(t *testing.T) {
		err := validateMinterAllowlist(txnWithToken("60001_5"), false,
			fakeGenesisLookup{minter: customMinter}, testLogger(), nil, true, false, true)
		if err != nil {
			t.Fatalf("expected the listed minter to pass, got: %v", err)
		}
	})

	t.Run("unlisted minter is rejected", func(t *testing.T) {
		err := validateMinterAllowlist(txnWithToken("60001_5"), false,
			fakeGenesisLookup{minter: otherMinter}, testLogger(), nil, true, false, true)
		if err == nil {
			t.Fatal("expected an unlisted minter to be rejected")
		}
		if !strings.Contains(err.Error(), "not authorised") {
			t.Fatalf("expected an authorisation error, got: %v", err)
		}
	})

	t.Run("token outside the minter range is rejected", func(t *testing.T) {
		err := validateMinterAllowlist(txnWithToken("60001_5000"), false,
			fakeGenesisLookup{minter: customMinter}, testLogger(), nil, true, false, true)
		if err == nil {
			t.Fatal("expected a token outside the range to be rejected")
		}
	})
}

// TestCustomNetworkWithoutMintersAcceptsEveryone covers an empty
// CustomNetAllowedMinters. Startup warns, and the check is skipped.
func TestCustomNetworkWithoutMintersAcceptsEveryone(t *testing.T) {
	withCustomMinters(t)

	err := validateMinterAllowlist(txnWithToken("60001_5"), false,
		fakeGenesisLookup{minter: otherMinter}, testLogger(), nil, true, false, true)
	if err != nil {
		t.Fatalf("expected an empty allowlist to accept any minter, got: %v", err)
	}
}

// TestRubixTestnetIgnoresCustomMinters is the guard on the default testnet: a
// false customNetwork means the Rubix swarm key, so the built in list applies
// and an outside minter is still rejected even when the custom list names it.
func TestRubixTestnetIgnoresCustomMinters(t *testing.T) {
	// Listed at the testnet level, so the only reason to accept it would be the
	// custom list being consulted. The token is a testnet one, so the rejection
	// comes from the minter lookup rather than the level check.
	withCustomMinters(t, minterallowlist.MintAccessRange{
		DID: customMinter, Level: constants.TestnetRBT_Level_Offset + 1, StartTokenNumber: 1, EndTokenNumber: 4300000,
	})

	err := validateMinterAllowlist(txnWithToken("50001_5"), false,
		fakeGenesisLookup{minter: customMinter}, testLogger(), nil, true, false, false)
	if err == nil {
		t.Fatal("expected the Rubix testnet list to reject an outside minter")
	}
	if !strings.Contains(err.Error(), "not authorised") {
		t.Fatalf("expected an authorisation error, got: %v", err)
	}
}

// TestMainnetNeverUsesCustomMinters is the property the whole design rests on.
// Even with the minter in the custom list and customNetwork set, mainnet
// validates against AllowedMinters and rejects.
func TestMainnetNeverUsesCustomMinters(t *testing.T) {
	withCustomMinters(t, minterallowlist.MintAccessRange{
		DID: customMinter, Level: 1, StartTokenNumber: 1, EndTokenNumber: 4300000,
	})

	err := validateMinterAllowlist(txnWithToken("1_5"), false,
		fakeGenesisLookup{minter: customMinter}, testLogger(), nil, false, true, true)
	if err == nil {
		t.Fatal("mainnet must never honour the custom minter list")
	}
	if !strings.Contains(err.Error(), "not authorised") {
		t.Fatalf("expected an authorisation error, got: %v", err)
	}
}

// TestLocalnetSkipsTheCheck confirms localnet behaviour is untouched.
func TestLocalnetSkipsTheCheck(t *testing.T) {
	err := validateMinterAllowlist(txnWithToken("10001_5"), false,
		fakeGenesisLookup{minter: otherMinter}, testLogger(), nil, false, false, false)
	if err != nil {
		t.Fatalf("localnet must skip the check, got: %v", err)
	}
}
