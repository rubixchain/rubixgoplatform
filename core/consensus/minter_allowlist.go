package consensus

import (
	"encoding/json"
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

// ValidateMinterAllowlist checks that every RBT in the transaction — both
// transferred and committed (split parents / SC-committed RBT) — was minted
// by an allowed DID. On fullnodes, pledged quorum tokens are checked too.
// For part tokens, it checks the whole-token ancestor.
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
	fetchGenesisTx func(peerDID, tokenID string) (*models.Transactions, error),
	testnet, mainnet bool,
) error {
	return validateMinterAllowlist(txnInfo, isFullnode, w, log, fetchGenesisTx, testnet, mainnet)
}

// validateMinterAllowlist is the test-friendly entry that takes an interface
// for the wallet lookup.
func validateMinterAllowlist(
	txnInfo *models.TransactionInfo,
	isFullnode bool,
	w genesisInitiatorLookup,
	log logger.Logger,
	fetchGenesisTx func(peerDID, tokenID string) (*models.Transactions, error),
	testnet, mainnet bool,
) error {
	if txnInfo == nil {
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
	case testnet:
		table = minterallowlist.TestnetAllowedMinters
		expectedLevel = 50001
	default:
		// Testnet and localnet: skip the check.
		_ = testnet
		return nil
	}

	// mintCheckToken pairs a token with the DID that actually holds its chain
	// history — needed because, for pledge tokens, that's the pledging quorum,
	// not the transaction initiator (mirrors syncFromPeerDID in checks.go's
	// TokenChainIntegrityCheck).
	type mintCheckToken struct {
		token       *models.TokenInfo
		genesisPeer string
	}

	tokensToCheck := make([]mintCheckToken, 0)
	if txnInfo.Tokens != nil {
		for _, t := range txnInfo.Tokens.RBT {
			tokensToCheck = append(tokensToCheck, mintCheckToken{token: t, genesisPeer: txnInfo.Initiator})
		}
	}
	for _, t := range txnInfo.CommittedTokens {
		tokensToCheck = append(tokensToCheck, mintCheckToken{token: t, genesisPeer: txnInfo.Initiator})
	}
	if isFullnode {
		for _, q := range txnInfo.Quorums {
			if q == nil {
				continue
			}
			for _, t := range q.Tokens {
				tokensToCheck = append(tokensToCheck, mintCheckToken{token: t, genesisPeer: q.Did})
			}
		}
	}
	for _, mc := range tokensToCheck {
		t := mc.token
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
		if lookupErr != nil && elems.PartIndex != 0 && fetchGenesisTx != nil {
			// Part-token transfer: the whole-token genesis may not be local yet.
			// Fetch ONLY the genesis transaction from the peer — this never
			// persists anything locally, so there's no risk of ingesting a
			// sibling transaction that's still being validated elsewhere.
			genesisTx, fetchErr := fetchGenesisTx(mc.genesisPeer, wholeID)
			if fetchErr != nil {
				return fmt.Errorf("ValidateMinterAllowlist: whole-token genesis fetch failed for %s (whole %s): %w",
					t.TokenID, wholeID, fetchErr)
			}
			var genesisInfo models.TransactionInfo
			if unmarshalErr := json.Unmarshal(genesisTx.Info, &genesisInfo); unmarshalErr != nil {
				return fmt.Errorf("ValidateMinterAllowlist: failed to unmarshal fetched genesis for %s (whole %s): %w",
					t.TokenID, wholeID, unmarshalErr)
			}
			if genesisInfo.Initiator == "" {
				lookupErr = fmt.Errorf("empty initiator in fetched genesis for whole %s", wholeID)
			} else {
				minter = genesisInfo.Initiator
				lookupErr = nil
			}
		}
		// Fallback: if local genesis lookup still fails and the current
		// transaction declares itself as the mint for this whole token
		// (PartIndex == 0 and PreviousTransactionID is empty), then the
		// transaction we are about to persist IS the genesis. In that case
		// the minter is the transaction initiator — no DB row exists yet
		// because the genesis chain row is created by this very transaction.
		if lookupErr != nil && elems.PartIndex == 0 && t.PreviousTransactionID == "" {
			if txnInfo.Initiator == "" {
				return fmt.Errorf("ValidateMinterAllowlist: genesis transaction for token %s has empty initiator", t.TokenID)
			}
			log.Debug("ValidateMinterAllowlist: local genesis missing; current transaction is the genesis, using initiator as minter",
				"tokenID", t.TokenID, "wholeID", wholeID, "initiator", txnInfo.Initiator)
			minter = txnInfo.Initiator
			lookupErr = nil
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
