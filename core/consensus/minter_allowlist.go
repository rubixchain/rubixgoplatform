package consensus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/core/minterallowlist"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// genesisInitiatorLookup is the set of wallet reads this gate uses. An interface
// so tests can swap in a fake.
//
// The height-0 reads back resolveSplitInitiator: for a part token, position 0 is
// the split transaction that created it, and its initiator is the DID to ask for
// the whole-token genesis.
type genesisInitiatorLookup interface {
	GetGenesisInitiatorDID(tokenID string, isFullNode bool) (string, error)
	GetTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error)
	GetFullNodeTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error)
}

// resolveSplitInitiator returns the DID that performed the split which created
// partTokenID, read from the part token's OWN position-0 chain entry.
//
// A split writes one transaction that both burns the parent (role Burn, appended
// to the parent's chain) and creates each child (role Mint, position 0 of the
// child's chain) — see wallet.PersistGenesisTransaction. So the part token's
// genesis IS the parent's burn, and its initiator is a DID that demonstrably
// held the whole token's chain at that moment.
//
// This read is local: TokenChainIntegrityCheck runs before this gate and syncs
// the transaction's tokens from position 0, and tokenchain rows carry a foreign
// key to transactions, so the row is present whenever the chain is. An error
// here is not fatal — the caller falls back to the peer it was given.
func resolveSplitInitiator(w genesisInitiatorLookup, partTokenID string, isFullnode bool) (string, error) {
	var (
		tx  *models.Transactions
		err error
	)
	if isFullnode {
		tx, _, err = w.GetFullNodeTransactionAndRoleAtHeight(partTokenID, 0)
	} else {
		tx, _, err = w.GetTransactionAndRoleAtHeight(partTokenID, 0)
	}
	if err != nil {
		return "", fmt.Errorf("split genesis unavailable locally for %s: %w", partTokenID, err)
	}
	if tx == nil {
		return "", fmt.Errorf("split genesis is nil for %s", partTokenID)
	}

	var info models.TransactionInfo
	if err := json.Unmarshal(tx.Info, &info); err != nil {
		return "", fmt.Errorf("unmarshal split genesis for %s: %w", partTokenID, err)
	}
	if info.Initiator == "" {
		return "", fmt.Errorf("split genesis for %s has empty initiator", partTokenID)
	}
	return info.Initiator, nil
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
	syncBurntChain func(peerDID, tokenID string) error,
	verifyGenesisSig func(signerDID string, info *models.TransactionInfo, signature string) error,
	testnet, mainnet bool,
) error {
	return validateMinterAllowlist(txnInfo, isFullnode, w, log, fetchGenesisTx, syncBurntChain, verifyGenesisSig, testnet, mainnet)
}

// enforceGenesisSignature gates whether a whole-token genesis whose signature
// fails to verify is REJECTED (true) or merely logged (false).
//
// Staged rollout, deliberately off. Verification needs the claimed minter's DID
// document, which Core.InitialiseDID may have to fetch over the network, so a
// benign resolution failure would reject an otherwise-valid mainnet transfer.
// Run log-only until telemetry shows no benign failures, then flip to true.
// The binding check (genesisMintsToken) is NOT gated — it is pure local
// computation and enforced unconditionally.
const enforceGenesisSignature = true

// genesisMintsToken reports whether info is genuinely the genesis of wholeID —
// that is, it mints that exact token with no predecessor.
//
// This binds a peer's answer to the question it was asked. Without it, the gate
// accepts any genesis naming an allowlisted minter, so a single genuine
// allowlisted mint replays as a universal pass for every part token on the
// network.
func genesisMintsToken(info *models.TransactionInfo, wholeID string) bool {
	if info == nil || info.Tokens == nil {
		return false
	}
	for _, gt := range info.Tokens.RBT {
		// PreviousTransactionID == "" is the genesis marker written by
		// Wallet.PersistGenesisTokenRecord.
		if gt != nil && gt.TokenID == wholeID && gt.PreviousTransactionID == "" {
			return true
		}
	}
	return false
}

// verifyFetchedGenesisSignature checks that the genesis carries the claimed
// minter's own signature over it.
//
// A whole-token mint sets Initiator == Owner == the minting DID and signs with
// util.SignTransaction (Wallet.PersistGenesisTokenRecord), so the initiator
// signature is exactly what util.VerifySignature validates. Passing this means
// the attacker must hold an allowlisted minter's private key, not merely name
// its DID.
func verifyFetchedGenesisSignature(
	tx *models.Transactions,
	info *models.TransactionInfo,
	verify func(signerDID string, info *models.TransactionInfo, signature string) error,
) error {
	if tx == nil || len(tx.Signature) == 0 {
		return fmt.Errorf("genesis transaction carries no signature")
	}
	var sig models.Signature
	if err := json.Unmarshal(tx.Signature, &sig); err != nil {
		return fmt.Errorf("unmarshal genesis signature: %w", err)
	}
	if sig.InitiatorSignature == "" {
		return fmt.Errorf("genesis has no initiator signature")
	}
	return verify(info.Initiator, info, sig.InitiatorSignature)
}

// validateMinterAllowlist is the test-friendly entry that takes an interface
// for the wallet lookup.
func validateMinterAllowlist(
	txnInfo *models.TransactionInfo,
	isFullnode bool,
	w genesisInitiatorLookup,
	log logger.Logger,
	fetchGenesisTx func(peerDID, tokenID string) (*models.Transactions, error),
	syncBurntChain func(peerDID, tokenID string) error,
	verifyGenesisSig func(signerDID string, info *models.TransactionInfo, signature string) error,
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
			// Part-token transfer: the whole-token genesis is not local, and for
			// a received part token it never will be — the parent is burnt at
			// split time and its chain is not propagated to part holders. Fetch
			// ONLY the genesis transaction from a peer; this never persists
			// anything locally, so there's no risk of ingesting a sibling
			// transaction that's still being validated elsewhere.
			//
			// Peer order matters. The split that created this part token burnt
			// the whole token, so whoever performed that split demonstrably held
			// the whole-token chain — ask them first. mc.genesisPeer (the current
			// transaction's initiator, or the pledging quorum) is merely the
			// latest holder; it has the whole-token chain only when it is itself
			// the splitter, i.e. on the first hop after a split, which is why
			// asking it alone fails on every later hop.
			var (
				peers      []string
				fetchErrs  []string
				servedPeer string
			)
			splitter, splitErr := resolveSplitInitiator(w, t.TokenID, isFullnode)
			if splitErr != nil {
				log.Debug("ValidateMinterAllowlist: could not resolve split initiator, falling back to declared peer",
					"tokenID", t.TokenID, "wholeID", wholeID, "err", splitErr)
			} else {
				peers = append(peers, splitter)
			}
			if mc.genesisPeer != "" && mc.genesisPeer != splitter {
				peers = append(peers, mc.genesisPeer)
			}

			for _, peerDID := range peers {
				genesisTx, fetchErr := fetchGenesisTx(peerDID, wholeID)
				if fetchErr != nil {
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: %v", peerDID, fetchErr))
					continue
				}
				if genesisTx == nil {
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: peer returned no genesis transaction", peerDID))
					continue
				}
				var genesisInfo models.TransactionInfo
				if unmarshalErr := json.Unmarshal(genesisTx.Info, &genesisInfo); unmarshalErr != nil {
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: unmarshal fetched genesis: %v", peerDID, unmarshalErr))
					continue
				}
				if genesisInfo.Initiator == "" {
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: empty initiator in fetched genesis", peerDID))
					continue
				}
				// Guard 1 — the peer was asked for wholeID's genesis; make it
				// prove that is what it served. Enforced unconditionally.
				if !genesisMintsToken(&genesisInfo, wholeID) {
					log.Error("ValidateMinterAllowlist: peer served a genesis for a different token",
						"tokenID", t.TokenID, "wholeID", wholeID, "peerDID", peerDID,
						"claimedMinter", genesisInfo.Initiator)
					fetchErrs = append(fetchErrs, fmt.Sprintf("%s: returned genesis does not mint %s", peerDID, wholeID))
					continue
				}
				// Guard 2 — the claimed minter must have signed it. Log-only
				// until enforceGenesisSignature is flipped; see that constant.
				if verifyGenesisSig != nil {
					if sigErr := verifyFetchedGenesisSignature(genesisTx, &genesisInfo, verifyGenesisSig); sigErr != nil {
						log.Error("ValidateMinterAllowlist: whole-token genesis signature did not verify",
							"tokenID", t.TokenID, "wholeID", wholeID, "peerDID", peerDID,
							"claimedMinter", genesisInfo.Initiator, "enforcing", enforceGenesisSignature,
							"err", sigErr)
						if enforceGenesisSignature {
							fetchErrs = append(fetchErrs, fmt.Sprintf("%s: genesis signature invalid: %v", peerDID, sigErr))
							continue
						}
					}
				}
				log.Debug("ValidateMinterAllowlist: resolved whole-token genesis from peer",
					"tokenID", t.TokenID, "wholeID", wholeID, "peerDID", peerDID)
				minter = genesisInfo.Initiator
				servedPeer = peerDID
				lookupErr = nil
				break
			}

			switch {
			case lookupErr == nil && syncBurntChain != nil:
				// Take the whole token's chain from the peer that just answered,
				// so the next part of the same token resolves from the local
				// chain above and needs no network at all. Only a burnt chain is
				// accepted — see Core.SyncBurntTokenChainFromPeer. A failure here
				// is not fatal: this transaction is already validated, and the
				// next one simply re-fetches.
				if syncErr := syncBurntChain(servedPeer, wholeID); syncErr != nil {
					log.Debug("ValidateMinterAllowlist: could not persist whole-token chain",
						"wholeID", wholeID, "peerDID", servedPeer, "err", syncErr)
				}
			case lookupErr != nil && len(fetchErrs) > 0:
				lookupErr = fmt.Errorf("whole-token genesis fetch failed from %d peer(s) [%s]",
					len(fetchErrs), strings.Join(fetchErrs, "; "))
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
