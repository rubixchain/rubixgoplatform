package consensus

import (
	"fmt"

	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// genesisInitiatorLookup is the wallet method this gate uses. An interface so
// tests can swap in a fake.
type genesisInitiatorLookup interface {
	GetGenesisInitiatorDID(tokenID string, isFullNode bool) (string, error)
}

// ValidateMinterAllowlist checks that every RBT in the transaction was minted
// by an allowed DID. For part tokens, it checks the whole-token ancestor.
//
// Enforced only on mainnet (uses AllowedMinters). Testnet enforcement is
// wired but disabled — see the switch below.
//
// Must run after TokenChainIntegrityCheck so the local chain is up to date.
// NFT, FT, and SmartContract entries are skipped.
func ValidateMinterAllowlist(
	txnInfo *models.TransactionInfo,
	isFullnode bool,
	w *wallet.Wallet,
	log logger.Logger,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
	testnet, mainnet bool,
) error {
	return validateMinterAllowlist(txnInfo, isFullnode, w, log, syncTxChains, testnet, mainnet)
}

// validateMinterAllowlist is the test-friendly entry that takes an interface
// for the wallet lookup.
func validateMinterAllowlist(
	txnInfo *models.TransactionInfo,
	isFullnode bool,
	w genesisInitiatorLookup,
	log logger.Logger,
	syncTxChains func(peerDID string, tokenIDs []string, prevTxIDs map[string]string, excludeTxIDs []string) error,
	testnet, mainnet bool,
) error {
	if txnInfo == nil || txnInfo.Tokens == nil {
		return nil
	}

	var (
		table         []minterallowlist.MintAccessRange
		expectedLevel int
	)
	switch {
	case mainnet:
		table = minterallowlist.AllowedMinters
		expectedLevel = 1
	// Testnet enforcement is currently disabled. Re-enable by uncommenting
	// the case below; TestnetAllowedMinters is still defined and tested.
	// case testnet:
	// 	table = minterallowlist.TestnetAllowedMinters
	// 	expectedLevel = 50001
	default:
		// Testnet and localnet: skip the check.
		_ = testnet
		return nil
	}

	for _, t := range txnInfo.Tokens.RBT {
		if t == nil || t.TokenID == "" {
			continue
		}
		elems, err := util.GetRbtIDElements(t.TokenID)
		if err != nil {
			return fmt.Errorf("ValidateMinterAllowlist: %w", err)
		}
		level, number := elems.TokenLevel, elems.TokenNumber
		if level != expectedLevel {
			return fmt.Errorf("ValidateMinterAllowlist: token %s has level %d; expected level %d for this network",
				t.TokenID, level, expectedLevel)
		}
		wholeID := fmt.Sprintf("%d_%d", level, number)

		minter, lookupErr := w.GetGenesisInitiatorDID(wholeID, isFullnode)
		if lookupErr != nil && elems.PartIndex != 0 && syncTxChains != nil {
			// Part-token transfer: the whole-token chain may not be local yet.
			// Pull it from the initiator and try the lookup again.
			log.Debug("ValidateMinterAllowlist: whole-token chain missing locally for part transfer, syncing from initiator",
				"partTokenID", t.TokenID, "wholeID", wholeID, "peerDID", txnInfo.Initiator)
			if syncErr := syncTxChains(
				txnInfo.Initiator,
				[]string{wholeID},
				map[string]string{wholeID: ""},
				nil,
			); syncErr != nil {
				return fmt.Errorf("ValidateMinterAllowlist: whole-token sync failed for %s (whole %s): %w",
					t.TokenID, wholeID, syncErr)
			}
			minter, lookupErr = w.GetGenesisInitiatorDID(wholeID, isFullnode)
		}
		if lookupErr != nil {
			log.Error("ValidateMinterAllowlist: cannot resolve genesis initiator",
				"tokenID", t.TokenID, "wholeID", wholeID, "err", lookupErr)
			return fmt.Errorf("ValidateMinterAllowlist: minter unverifiable for token %s (whole %s): %w",
				t.TokenID, wholeID, lookupErr)
		}

		if !minterallowlist.ValidateMinterAuthorization(table, minter, level, number) {
			log.Error("ValidateMinterAllowlist: rejection",
				"tokenID", t.TokenID, "wholeID", wholeID,
				"minter", minter, "level", level, "tokenNumber", number)
			return fmt.Errorf("ValidateMinterAllowlist: token %s minter %s not authorised for level %d number %d",
				t.TokenID, minter, level, number)
		}
	}
	return nil
}
