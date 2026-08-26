package consensus

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

const (
	testNFTID       = "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco"
	testPropsID     = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	testDeployer    = "bafybmideployerdid"
	testWhitelisted = "bafybmiwhitelisteddid"
	testOutsider    = "bafybmioutsiderdid"
)

// nftTxn builds a minimal NFT transaction.
func nftTxn(initiator string, epoch int) *models.TransactionInfo {
	return &models.TransactionInfo{
		Initiator: initiator,
		Epoch:     epoch,
		Network:   "testnet",
		Tokens: &models.TransactionTokens{
			NFT: []*models.TokenInfo{{TokenID: testNFTID, PreviousTransactionID: "prev"}},
		},
	}
}

// resolverReturning yields a fixed resolver result.
func resolverReturning(res *models.ResolvedProperties, err error) func(string) (*models.ResolvedProperties, error) {
	return func(string) (*models.ResolvedProperties, error) { return res, err }
}

// props builds a resolved document.
func props(flags uint32, whitelist []string) *models.ResolvedProperties {
	return &models.ResolvedProperties{
		PropertiesTokenID: testPropsID,
		DocCID:            testPropsID,
		Doc:               &models.TokenProperties{Version: models.PropertiesDocVersion, Flags: flags},
		Whitelist:         whitelist,
		Deployer:          testDeployer,
	}
}

func TestValidateNFTProperties_AbsentIsUnrestricted(t *testing.T) {
	err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil, resolverReturning(nil, nil))
	if err != nil {
		t.Fatalf("absent properties must allow the transaction, got: %v", err)
	}
}

func TestValidateNFTProperties_ResolveFailureRejects(t *testing.T) {
	err := ValidateNFTProperties(nftTxn(testWhitelisted, 1000), false, nil,
		resolverReturning(nil, fmt.Errorf("peer unreachable")))
	if err == nil {
		t.Fatal("an unresolvable properties document must reject, not allow")
	}
	if !strings.Contains(err.Error(), "peer unreachable") {
		t.Errorf("expected the resolve error to propagate, got: %v", err)
	}
}

func TestValidateNFTProperties_NilResolverRejects(t *testing.T) {
	if err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil, nil); err == nil {
		t.Fatal("a missing resolver must reject rather than skip enforcement")
	}
}

func TestValidateNFTProperties_Whitelist(t *testing.T) {
	wl := []string{testWhitelisted, testDeployer}

	if err := ValidateNFTProperties(nftTxn(testWhitelisted, 1000), false, nil,
		resolverReturning(props(0, wl), nil)); err != nil {
		t.Errorf("whitelisted initiator must be allowed, got: %v", err)
	}

	err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil,
		resolverReturning(props(0, wl), nil))
	if err == nil {
		t.Fatal("non-whitelisted initiator must be rejected")
	}
	if !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("expected a whitelist error, got: %v", err)
	}

	// An empty whitelist is unrestricted.
	if err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil,
		resolverReturning(props(0, nil), nil)); err != nil {
		t.Errorf("empty whitelist must be unrestricted, got: %v", err)
	}
}

func TestValidateNFTProperties_Transferable(t *testing.T) {
	wl := []string{testWhitelisted}

	err := ValidateNFTProperties(nftTxn(testWhitelisted, 1000), true, nil,
		resolverReturning(props(0, wl), nil))
	if err == nil {
		t.Fatal("transfer must be rejected when the transferable flag is unset")
	}
	if !strings.Contains(err.Error(), "non-transferable") {
		t.Errorf("expected a transferable error, got: %v", err)
	}

	if err := ValidateNFTProperties(nftTxn(testWhitelisted, 1000), true, nil,
		resolverReturning(props(models.FlagTransferable, wl), nil)); err != nil {
		t.Errorf("transfer must be allowed when the flag is set, got: %v", err)
	}

	// The same document permits a non-transfer execute.
	if err := ValidateNFTProperties(nftTxn(testWhitelisted, 1000), false, nil,
		resolverReturning(props(0, wl), nil)); err != nil {
		t.Errorf("execute must be unaffected by the transferable flag, got: %v", err)
	}
}

// Uses the signed Epoch, so nodes with skewed clocks reach the same verdict.
func TestValidateNFTProperties_ValidityWindowUsesEpoch(t *testing.T) {
	windowed := func() *models.ResolvedProperties {
		r := props(0, nil)
		r.Doc.Policy = models.PropertiesPolicy{ValidFrom: 1000, ValidTo: 2000}
		return r
	}

	for _, tc := range []struct {
		name    string
		epoch   int
		allowed bool
	}{
		{"before window", 999, false},
		{"on lower bound", 1000, true},
		{"inside", 1500, true},
		{"on upper bound", 2000, true},
		{"after window", 2001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNFTProperties(nftTxn(testOutsider, tc.epoch), false, nil,
				resolverReturning(windowed(), nil))
			if tc.allowed && err != nil {
				t.Errorf("epoch %d should be allowed, got: %v", tc.epoch, err)
			}
			if !tc.allowed && err == nil {
				t.Errorf("epoch %d should be rejected", tc.epoch)
			}
		})
	}
}

func TestValidateNFTProperties_RestrictionsCombineAsAnd(t *testing.T) {
	r := props(0, []string{testWhitelisted})
	r.Doc.Policy = models.PropertiesPolicy{ValidFrom: 1000, ValidTo: 2000}

	// Whitelisted but outside the window.
	if err := ValidateNFTProperties(nftTxn(testWhitelisted, 5000), false, nil,
		resolverReturning(r, nil)); err == nil {
		t.Error("whitelisted initiator outside the validity window must still be rejected")
	}
	// Inside the window but not whitelisted.
	if err := ValidateNFTProperties(nftTxn(testOutsider, 1500), false, nil,
		resolverReturning(r, nil)); err == nil {
		t.Error("in-window initiator not on the whitelist must still be rejected")
	}
	// Both satisfied.
	if err := ValidateNFTProperties(nftTxn(testWhitelisted, 1500), false, nil,
		resolverReturning(r, nil)); err != nil {
		t.Errorf("both restrictions satisfied should be allowed, got: %v", err)
	}
}

func TestValidateNFTProperties_AllowedSubnets(t *testing.T) {
	r := props(0, nil)
	r.Doc.Restriction.AllowedSubnets = []string{"mainnet"}

	if err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil,
		resolverReturning(r, nil)); err == nil {
		t.Error("a transaction on testnet must be rejected when only mainnet is allowed")
	}

	r.Doc.Restriction.AllowedSubnets = []string{"testnet"}
	if err := ValidateNFTProperties(nftTxn(testOutsider, 1000), false, nil,
		resolverReturning(r, nil)); err != nil {
		t.Errorf("matching subnet must be allowed, got: %v", err)
	}
}

func TestValidateNFTProperties_AllowedSmartContracts(t *testing.T) {
	r := props(0, nil)
	r.Doc.Restriction.AllowedSmartContracts = []string{"QmAllowedContract"}

	txn := nftTxn(testOutsider, 1000)
	txn.Tokens.SmartContract = []*models.TokenInfo{{TokenID: "QmOtherContract"}}
	if err := ValidateNFTProperties(txn, false, nil, resolverReturning(r, nil)); err == nil {
		t.Error("a smart contract outside the allowlist must be rejected")
	}

	txn.Tokens.SmartContract = []*models.TokenInfo{{TokenID: "QmAllowedContract"}}
	if err := ValidateNFTProperties(txn, false, nil, resolverReturning(r, nil)); err != nil {
		t.Errorf("an allowlisted smart contract must be permitted, got: %v", err)
	}
}

func TestValidateNFTProperties_EditAuthorization(t *testing.T) {
	edit := func(initiator, prevTxID string) *models.TransactionInfo {
		txn := nftTxn(initiator, 1000)
		txn.Tokens.Properties = []*models.TokenInfo{
			{TokenID: testPropsID, PreviousTransactionID: prevTxID, Data: testPropsID},
		}
		return txn
	}

	if err := ValidateNFTProperties(edit(testDeployer, "prevprops"), false, nil,
		resolverReturning(props(0, nil), nil)); err != nil {
		t.Errorf("the deployer must be allowed to edit, got: %v", err)
	}

	err := ValidateNFTProperties(edit(testWhitelisted, "prevprops"), false, nil,
		resolverReturning(props(0, nil), nil))
	if err == nil {
		t.Fatal("a non-deployer must not be allowed to edit properties")
	}
	if !strings.Contains(err.Error(), "only the deployer") {
		t.Errorf("expected a deployer authorization error, got: %v", err)
	}

	// Genesis: no PreviousTransactionID, so no deployer exists yet.
	if err := ValidateNFTProperties(edit(testWhitelisted, ""), false, nil,
		resolverReturning(props(0, nil), nil)); err != nil {
		t.Errorf("genesis properties creation must be allowed, got: %v", err)
	}
}

func TestValidateNFTProperties_EditMustGovernNamedNFT(t *testing.T) {
	txn := nftTxn(testDeployer, 1000)
	txn.Tokens.Properties = []*models.TokenInfo{
		{TokenID: "QmUnrelatedPropsToken", PreviousTransactionID: "prevprops"},
	}

	err := ValidateNFTProperties(txn, false, nil, resolverReturning(props(0, nil), nil))
	if err == nil {
		t.Fatal("a properties token governing no NFT in the transaction must be rejected")
	}
	if !strings.Contains(err.Error(), "does not govern") {
		t.Errorf("expected a governance error, got: %v", err)
	}
}
