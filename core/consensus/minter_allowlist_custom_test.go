package consensus

import (
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

const (
	customMinter = "bafybmicustomminter"
	otherMinter  = "bafybmisomeoneelse"
	// customLevel is level 1 of the custom network series.
	customLevel = constants.CustomNetRBT_Level_Offset + 1
)

// onCustomNetwork marks the node as being on a custom network for the duration
// of the test, and points CustomNetAllowedMinters at the given list. Both are
// restored when the test ends, so the Rubix network tests are unaffected.
func onCustomNetwork(t *testing.T, minters ...minterallowlist.MintAccessRange) {
	t.Helper()
	savedMinters := minterallowlist.CustomNetAllowedMinters
	minterallowlist.CustomNetAllowedMinters = minters
	minterallowlist.SetCustomNetwork(true)
	t.Cleanup(func() {
		minterallowlist.CustomNetAllowedMinters = savedMinters
		minterallowlist.SetCustomNetwork(false)
	})
}

// customLookup resolves tokenID's whole-token genesis to minter. It reuses the
// fakeGenesisLookup in minter_allowlist_test.go, so the custom network tests
// exercise the same stub as the Rubix network ones.
func customLookup(tokenID, minter string) *fakeGenesisLookup {
	return &fakeGenesisLookup{
		genesisInitiator: map[string]string{tokenID: minter},
	}
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

// TestCustomNetworkUsesItsOwnMinters covers the case the gate exists for: a
// testnet node on a non-Rubix swarm validates against CustomNetAllowedMinters.
func TestCustomNetworkUsesItsOwnMinters(t *testing.T) {
	listed := minterallowlist.MintAccessRange{
		DID: customMinter, Level: customLevel, StartTokenNumber: 1, EndTokenNumber: 1000,
	}

	t.Run("listed minter passes", func(t *testing.T) {
		onCustomNetwork(t, listed)
		err := validateMinterAllowlist(txnWithToken("60001_5"), false,
			customLookup("60001_5", customMinter), testLogger(), nil, nil, nil, true, false)
		if err != nil {
			t.Fatalf("expected the listed minter to pass, got: %v", err)
		}
	})

	t.Run("unlisted minter is rejected", func(t *testing.T) {
		onCustomNetwork(t, listed)
		err := validateMinterAllowlist(txnWithToken("60001_5"), false,
			customLookup("60001_5", otherMinter), testLogger(), nil, nil, nil, true, false)
		if err == nil {
			t.Fatal("expected an unlisted minter to be rejected")
		}
		if !strings.Contains(err.Error(), "not authorised") {
			t.Fatalf("expected an authorisation error, got: %v", err)
		}
	})

	t.Run("token outside the minter range is rejected", func(t *testing.T) {
		onCustomNetwork(t, listed)
		err := validateMinterAllowlist(txnWithToken("60001_5000"), false,
			customLookup("60001_5000", customMinter), testLogger(), nil, nil, nil, true, false)
		if err == nil {
			t.Fatal("expected a token outside the range to be rejected")
		}
	})
}

// TestCustomNetworkWithoutMintersAcceptsEveryone covers an empty
// CustomNetAllowedMinters. Startup warns, and the check is skipped.
func TestCustomNetworkWithoutMintersAcceptsEveryone(t *testing.T) {
	onCustomNetwork(t)

	err := validateMinterAllowlist(txnWithToken("60001_5"), false,
		customLookup("60001_5", otherMinter), testLogger(), nil, nil, nil, true, false)
	if err != nil {
		t.Fatalf("expected an empty allowlist to accept any minter, got: %v", err)
	}
}

// TestRubixTestnetIgnoresCustomMinters is the guard on the default testnet: with
// the custom network flag unset, the built in list applies and an outside minter
// is rejected even though the custom list names it.
func TestRubixTestnetIgnoresCustomMinters(t *testing.T) {
	savedMinters := minterallowlist.CustomNetAllowedMinters
	minterallowlist.CustomNetAllowedMinters = []minterallowlist.MintAccessRange{
		{DID: customMinter, Level: constants.TestnetRBT_Level_Offset + 1, StartTokenNumber: 1, EndTokenNumber: 4300000},
	}
	t.Cleanup(func() { minterallowlist.CustomNetAllowedMinters = savedMinters })

	err := validateMinterAllowlist(txnWithToken("50001_5"), false,
		customLookup("50001_5", customMinter), testLogger(), nil, nil, nil, true, false)
	if err == nil {
		t.Fatal("expected the Rubix testnet list to reject an outside minter")
	}
	if !strings.Contains(err.Error(), "not authorised") {
		t.Fatalf("expected an authorisation error, got: %v", err)
	}
}

// TestMainnetNeverUsesCustomMinters is the property the whole design rests on.
// Even with the flag set and the minter in the custom list, mainnet validates
// against AllowedMinters and rejects.
func TestMainnetNeverUsesCustomMinters(t *testing.T) {
	onCustomNetwork(t, minterallowlist.MintAccessRange{
		DID: customMinter, Level: 1, StartTokenNumber: 1, EndTokenNumber: 4300000,
	})

	err := validateMinterAllowlist(txnWithToken("1_5"), false,
		customLookup("1_5", customMinter), testLogger(), nil, nil, nil, false, true)
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
		customLookup("10001_5", otherMinter), testLogger(), nil, nil, nil, false, false)
	if err != nil {
		t.Fatalf("localnet must skip the check, got: %v", err)
	}
}

// TestCustomNetworkTokenLevels covers the level offset in ValidateNewTokenContent.
// A custom network runs in testnet mode, so testnet is set in every case.
func TestCustomNetworkTokenLevels(t *testing.T) {
	log := testLogger()

	t.Run("custom level accepted on a custom network", func(t *testing.T) {
		onCustomNetwork(t)
		if err := ValidateNewTokenContent("60001_1", false, true, false, false, log); err != nil {
			t.Fatalf("expected a custom network token to pass, got: %v", err)
		}
	})

	t.Run("testnet level rejected on a custom network", func(t *testing.T) {
		onCustomNetwork(t)
		err := ValidateNewTokenContent("50001_1", false, true, false, false, log)
		if err == nil {
			t.Fatal("expected a testnet level to be rejected on a custom network")
		}
		if !strings.Contains(err.Error(), "custom network level must be >=") {
			t.Fatalf("expected a custom level error, got: %v", err)
		}
	})

	t.Run("custom level rejected on the Rubix testnet", func(t *testing.T) {
		err := ValidateNewTokenContent("60001_1", false, true, false, false, log)
		if err == nil {
			t.Fatal("expected a custom level to be rejected on the Rubix testnet")
		}
		if !strings.Contains(err.Error(), "not present in TokenMap") {
			t.Fatalf("expected a TokenMap error, got: %v", err)
		}
	})

	t.Run("testnet level still accepted on the Rubix testnet", func(t *testing.T) {
		if err := ValidateNewTokenContent("50001_1", false, true, false, false, log); err != nil {
			t.Fatalf("the Rubix testnet must be unaffected, got: %v", err)
		}
	})
}
