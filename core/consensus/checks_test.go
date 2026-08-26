package consensus

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// validDID builds a DID that satisfies validateDID:
//   - alphanumeric
//   - has DidPrefix ("bafybmi")
//   - length == DidLength (59)
//
// A seed char is repeated to fill the suffix so callers can generate distinct
// DIDs (e.g. initiator vs owner vs quorum) without building literals by hand.
func validDID(seed byte) string {
	suffixLen := constants.DidLength - len(constants.DidPrefix)
	return constants.DidPrefix + strings.Repeat(string(seed), suffixLen)
}

// validEpoch returns an epoch that is after MinTransactionEpochUnix but before
// time.Now(). MinTransactionEpochUnix corresponds to 2026-04-01 UTC.
func validEpoch() int {
	return constants.MinTransactionEpochUnix + 60
}

// baseValidTxnInfo returns a TransactionInfo that passes
// ValidateTransactionInfoFields. Individual test rows can clone it and mutate
// the single field being exercised.
func baseValidTxnInfo() models.TransactionInfo {
	return models.TransactionInfo{
		Initiator: validDID('a'),
		Owner:     validDID('b'),
		Epoch:     validEpoch(),
		Network:   constants.NetworkMode_Mainnet,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{
				{TokenID: "1_1", PreviousTransactionID: "", TokenValue: 1.0},
			},
		},
	}
}

// stubDIDCrypto is an in-memory fake that satisfies types.DIDCrypto.
// Tests set Verify / VerifyErr to shape responses per-case.
type stubDIDCrypto struct {
	did       string
	Verify    bool
	VerifyErr error
	SignBytes []byte
	SignErr   error
}

func (s *stubDIDCrypto) GetDID() string                            { return s.did }
func (s *stubDIDCrypto) Sign(hash []byte) ([]byte, error)          { return s.SignBytes, s.SignErr }
func (s *stubDIDCrypto) SignVerify(hash, sig []byte) (bool, error) { return s.Verify, s.VerifyErr }

// -----------------------------------------------------------------------------
// ValidateTransactionInfoFields:
// Covers: bad InitiatorDID/OwnerDID, bad Epoch, bad Network, nil/empty Tokens.
// -----------------------------------------------------------------------------

func TestValidateTransactionInfoFields(t *testing.T) {
	now := int(time.Now().Unix())

	tests := []struct {
		name              string
		mutate            func(*models.TransactionInfo)
		configuredNetwork string
		wantErr           bool
		wantMatch         string // substring expected in the error (optional)
	}{
		// ---- happy path ----
		{name: "valid input", mutate: func(*models.TransactionInfo) {}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: false},

		// ---- Initiator DID ----
		{name: "initiator empty", mutate: func(tx *models.TransactionInfo) { tx.Initiator = "" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "initiator"},
		{name: "initiator with non-alphanumeric", mutate: func(tx *models.TransactionInfo) { tx.Initiator = "bafybmi!!!" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "alphanumeric"},
		{name: "initiator wrong prefix", mutate: func(tx *models.TransactionInfo) {
			tx.Initiator = "xxxxxxx" + strings.Repeat("a", constants.DidLength-7)
		}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "must start with"},
		{name: "initiator wrong length (short)", mutate: func(tx *models.TransactionInfo) {
			tx.Initiator = constants.DidPrefix + "a"
		}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "length"},
		{name: "initiator wrong length (long)", mutate: func(tx *models.TransactionInfo) {
			tx.Initiator = constants.DidPrefix + strings.Repeat("a", constants.DidLength) // too long
		}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "length"},

		// ---- Owner DID ----
		{name: "owner empty is allowed (deployment txns)", mutate: func(tx *models.TransactionInfo) { tx.Owner = "" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: false},
		{name: "owner has space", mutate: func(tx *models.TransactionInfo) { tx.Owner = "bafybmi abc" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "alphanumeric"},

		// ---- Epoch ----
		{name: "epoch zero", mutate: func(tx *models.TransactionInfo) { tx.Epoch = 0 },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "epoch"},
		{name: "epoch negative", mutate: func(tx *models.TransactionInfo) { tx.Epoch = -1 },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "epoch"},
		{name: "epoch before April-1-2026", mutate: func(tx *models.TransactionInfo) {
			tx.Epoch = constants.MinTransactionEpochUnix - 1
		}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "epoch"},
		{name: "epoch in the future", mutate: func(tx *models.TransactionInfo) { tx.Epoch = now + 3600 },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "epoch"},

		// ---- Network ----
		{name: "network empty", mutate: func(tx *models.TransactionInfo) { tx.Network = "" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "network"},
		{name: "network unknown", mutate: func(tx *models.TransactionInfo) { tx.Network = "devnet" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "network"},
		{name: "network wrong case", mutate: func(tx *models.TransactionInfo) { tx.Network = "MAINNET" },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "network"},
		{name: "network testnet OK", mutate: func(tx *models.TransactionInfo) {
			tx.Network = constants.NetworkMode_Testnet
		}, configuredNetwork: constants.NetworkMode_Testnet, wantErr: false},
		{name: "network localnet OK", mutate: func(tx *models.TransactionInfo) {
			tx.Network = constants.NetworkMode_Localnet
		}, configuredNetwork: constants.NetworkMode_Localnet, wantErr: false},
		{name: "network mismatch with configured mainnet", mutate: func(tx *models.TransactionInfo) {
			tx.Network = constants.NetworkMode_Testnet
		}, configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "network mismatch"},
		{name: "invalid configured network mode", mutate: func(*models.TransactionInfo) {},
			configuredNetwork: "devnet", wantErr: true, wantMatch: "configured network mode"},

		// ---- Tokens ----
		{name: "tokens pointer is nil", mutate: func(tx *models.TransactionInfo) { tx.Tokens = nil },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "transfer token"},
		{name: "tokens pointer set but all lists empty",
			mutate:            func(tx *models.TransactionInfo) { tx.Tokens = &models.TransactionTokens{} },
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: true, wantMatch: "transfer token"},
		{name: "only NFT populated is OK",
			mutate: func(tx *models.TransactionInfo) {
				tx.Tokens = &models.TransactionTokens{NFT: []*models.TokenInfo{{TokenID: "nft1"}}}
			},
			configuredNetwork: constants.NetworkMode_Mainnet, wantErr: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tx := baseValidTxnInfo()
			tc.mutate(&tx)
			err := ValidateTransactionInfoFields(&tx, tc.configuredNetwork)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.wantMatch != "" && !strings.Contains(err.Error(), tc.wantMatch) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantMatch)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TransactionIDIntegrityCheck:
// Covers: any tampering of txInfo after the ID was computed.
// -----------------------------------------------------------------------------

func TestTransactionIDIntegrityCheck(t *testing.T) {
	tx := baseValidTxnInfo()
	correctID, err := util.GetTransactionID(&tx)
	if err != nil {
		t.Fatalf("GetTransactionID: %v", err)
	}

	t.Run("matching ID passes", func(t *testing.T) {
		if err := TransactionIDIntegrityCheck(correctID, &tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("tampered ID fails", func(t *testing.T) {
		if err := TransactionIDIntegrityCheck("deadbeef", &tx); err == nil {
			t.Fatal("expected mismatch error, got nil")
		}
	})

	t.Run("mutating any field invalidates the ID", func(t *testing.T) {
		txCopy := tx
		txCopy.Memo = "changed after signing"
		if err := TransactionIDIntegrityCheck(correctID, &txCopy); err == nil {
			t.Fatal("expected mismatch error after field mutation, got nil")
		}
	})
}

// -----------------------------------------------------------------------------
// SignatureVerificationCheck:
// Covers: empty / bad initiator sig, missing quorum DC, bad quorum sig.
// -----------------------------------------------------------------------------

func TestSignatureVerificationCheck(t *testing.T) {
	tx := baseValidTxnInfo()

	quorumDID := validDID('q')

	// A quorum signature payload (base64("ok")) that a passing DC will accept.
	goodQuorumSig := util.BytesToBase64([]byte("ok"))

	tests := []struct {
		name     string
		sig      *models.Signature
		initDC   *stubDIDCrypto
		quorums  map[string]types.DIDCrypto
		wantErr  bool
		errMatch string
	}{
		{
			name:    "Only initiator signature, no quorum involves",
			sig:     &models.Signature{InitiatorSignature: util.BytesToBase64([]byte("x"))},
			initDC:  &stubDIDCrypto{Verify: true},
			quorums: map[string]types.DIDCrypto{},
			wantErr: false,
		},
		{
			name:     "empty initiator signature returns !ok",
			sig:      &models.Signature{InitiatorSignature: ""}, // decodes to empty []byte, no decode err
			initDC:   &stubDIDCrypto{Verify: false},
			quorums:  map[string]types.DIDCrypto{},
			wantErr:  true,
			errMatch: "initiator signature",
		},
		{
			name:     "bad base64 initiator signature fails to decode",
			sig:      &models.Signature{InitiatorSignature: "!!!not-base64!!!"},
			initDC:   &stubDIDCrypto{Verify: true},
			quorums:  map[string]types.DIDCrypto{},
			wantErr:  true,
			errMatch: "decode",
		},
		{
			name:     "initiator DC reports verify error",
			sig:      &models.Signature{InitiatorSignature: util.BytesToBase64([]byte("x"))},
			initDC:   &stubDIDCrypto{Verify: false, VerifyErr: errors.New("boom")},
			quorums:  map[string]types.DIDCrypto{},
			wantErr:  true,
			errMatch: "boom",
		},
		{
			name: "missing DIDCrypto for quorum",
			sig: &models.Signature{
				InitiatorSignature: util.BytesToBase64([]byte("x")),
				Quorums:            []models.QuorumSignature{{Did: quorumDID, Signature: goodQuorumSig}},
			},
			initDC:   &stubDIDCrypto{Verify: true},
			quorums:  map[string]types.DIDCrypto{}, // quorumDID not provided
			wantErr:  true,
			errMatch: "no DIDCrypto provided",
		},
		{
			name: "quorum signature invalid",
			sig: &models.Signature{
				InitiatorSignature: util.BytesToBase64([]byte("x")),
				Quorums:            []models.QuorumSignature{{Did: quorumDID, Signature: goodQuorumSig}},
			},
			initDC: &stubDIDCrypto{Verify: true},
			quorums: map[string]types.DIDCrypto{
				quorumDID: &stubDIDCrypto{Verify: false},
			},
			wantErr:  true,
			errMatch: "quorum " + quorumDID,
		},
		{
			name: "all signatures valid",
			sig: &models.Signature{
				InitiatorSignature: util.BytesToBase64([]byte("x")),
				Quorums:            []models.QuorumSignature{{Did: quorumDID, Signature: goodQuorumSig}},
			},
			initDC: &stubDIDCrypto{Verify: true},
			quorums: map[string]types.DIDCrypto{
				quorumDID: &stubDIDCrypto{Verify: true},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := SignatureVerificationCheck(&tx, tc.sig, tc.initDC, tc.quorums)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errMatch)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ValidateIPFSPinChecks:
// Uses an injected closure, so it is trivial to test without IPFS.
// -----------------------------------------------------------------------------

func TestValidateIPFSPinChecks(t *testing.T) {
	tx := baseValidTxnInfo()
	tx.Tokens.RBT = []*models.TokenInfo{
		{TokenID: "ok_token", PreviousTransactionID: "prev1"},
		{TokenID: "pinned_token", PreviousTransactionID: "prev2"},
	}

	// Simulate: any token named "pinned_token" is already pinned → should error.
	checkPinned := func(tokenID, prevTxnID string) error {
		if tokenID == "pinned_token" {
			return errors.New("already pinned")
		}
		return nil
	}

	err := ValidateIPFSPinChecks(&tx, false, checkPinned)
	if err == nil {
		t.Fatal("expected pin error, got nil")
	}

	// All clear path.
	okPinned := func(string, string) error { return nil }
	if err := ValidateIPFSPinChecks(&tx, false, okPinned); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// ValidateTransactionValueAndPledge:
// Covers: pledge must be >= RBT transaction value when quorums involved.
// NOTE: This test uses tokenIDs that util.GetTokenValueFromTokenID can parse.
// Adjust the IDs per your TokenMap / network offsets as needed.
// -----------------------------------------------------------------------------

func TestValidateTransactionValueAndPledge(t *testing.T) {
	t.Run("no quorum is a no-op", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Quorums = nil
		if err := ValidateTransactionValueAndPledge(&tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nil tokens errors", func(t *testing.T) {
		tx := baseValidTxnInfo()
		tx.Tokens = nil
		if err := ValidateTransactionValueAndPledge(&tx); err == nil {
			t.Fatal("expected error for nil tokens")
		}
	})
}

// -----------------------------------------------------------------------------
// Pure helper tests (no DB needed).
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// Orchestrator-level (ValidateTransaction) sanity tests.
//
// These fail BEFORE any wallet/IPFS call is reached, so they do not need a
// real Postgres or IPFS. For DB-dependent paths (TokenChainIntegrityCheck,
// ValidateTokenOwnershipByPrevTxn, IsParentTokenBurnt) you should either:
//  1. refactor those checks to accept a small interface, then mock; or
//  2. use testcontainers-go to spin up an ephemeral Postgres.
// -----------------------------------------------------------------------------

func TestValidateTransaction_FailsOnInvalidInfoFields(t *testing.T) {
	tx := baseValidTxnInfo()
	tx.Epoch = -1 // invalid

	infoBytes, _ := json.Marshal(&tx)
	sigBytes, _ := json.Marshal(&models.Signature{})

	// TxID doesn't matter — we fail before the ID check too, but supply a
	// well-formed struct so json.Unmarshal in ValidateTransaction succeeds.
	storedTx := &models.Transactions{
		ID:        "anything",
		Info:      infoBytes,
		Signature: sigBytes,
	}

	_, err := ValidateTransaction(
		storedTx,
		false, // isFullnode
		nil,   // wallet – never touched in this failure path
		nil,   // logger – never touched in this failure path
		&stubDIDCrypto{Verify: true},
		map[string]types.DIDCrypto{},
		false, true, false,
		func(string, string) error { return nil },
		func(string, []string, map[string]string, []string) error { return nil },
		func([]string) (map[string]string, error) { return map[string]string{}, nil }, // syncAuthoritative
		func(string) (*models.TransactionInfo, error) { return nil, nil }, // getTxByID
		func(string) (string, bool, error) { return "", false, nil },      // getParentBurnTx
		func(string, string) (*models.Transactions, error) { return nil, nil },
		false, // transferNFTOwnership
	)
	if err == nil {
		t.Fatal("expected error because of invalid epoch, got nil")
	}
	if !strings.Contains(err.Error(), "epoch") {
		t.Fatalf("expected epoch error, got: %v", err)
	}
}

func TestValidateTransaction_FailsOnTxIDMismatch(t *testing.T) {
	tx := baseValidTxnInfo()
	infoBytes, _ := json.Marshal(&tx)
	sigBytes, _ := json.Marshal(&models.Signature{})

	storedTx := &models.Transactions{
		ID:        "does-not-match",
		Info:      infoBytes,
		Signature: sigBytes,
	}

	_, err := ValidateTransaction(
		storedTx, false, nil, nil,
		&stubDIDCrypto{Verify: true},
		map[string]types.DIDCrypto{},
		false, true, false,
		func(string, string) error { return nil },
		func(string, []string, map[string]string, []string) error { return nil },
		func([]string) (map[string]string, error) { return map[string]string{}, nil }, // syncAuthoritative
		func(string) (*models.TransactionInfo, error) { return nil, nil }, // getTxByID
		func(string) (string, bool, error) { return "", false, nil },      // getParentBurnTx
		func(string, string) (*models.Transactions, error) { return nil, nil },
		false, // transferNFTOwnership
	)
	if err == nil || !strings.Contains(err.Error(), "transaction ID mismatch") {
		t.Fatalf("expected tx ID mismatch error, got: %v", err)
	}
}

// =============================================================================
// ValidateSplitParentsAgainstFullnode — replayed-RBT-split double-spend.
//
// The check derives each freshly-split child's ancestors (util TokenID
// GetHierarchy) and, for each, compares the authoritative burn tx against the
// child's minting genesis. Tests use a real hierarchical child so the derivation
// is genuine: 1_1_14 -> ancestors [1_1_3, 1_1_1, 1_1]. Driven purely through the
// injected callbacks; the *wallet.Wallet arg is unused, passed nil.
// =============================================================================

func TestValidateSplitParentsAgainstFullnode(t *testing.T) {
	const (
		genTX   = "genesis-split-tx-1"   // the split that mints the child now
		otherTX = "genesis-split-tx-old" // an earlier different split (the replay's true burn)
		childID = "1_1_14"               // freshly-minted child; ancestors: 1_1_3, 1_1_1, 1_1
	)
	// ancestors of childID, deepest→whole (must match util GetHierarchy output).
	ancImmediate := "1_1_3"
	ancMid := "1_1_1"
	ancWhole := "1_1"

	// A transfer of a freshly-split child: RBT token whose prev is the split genesis.
	transferInfo := func() *models.TransactionInfo {
		return &models.TransactionInfo{
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{
					{TokenID: childID, PreviousTransactionID: genTX, TokenValue: 0.001},
				},
			},
		}
	}
	// genTX is a split genesis. Its CommittedTokens declares which ancestors it
	// burns; the check intersects this with the derived ancestors. We list all
	// three ancestors with EMPTY prev to prove the check does NOT depend on the
	// prev field (the old evasion) — only on the fullnode's burn-by-whom.
	genInfo := &models.TransactionInfo{
		CommittedTokens: []*models.TokenInfo{
			{TokenID: ancImmediate, PreviousTransactionID: ""},
			{TokenID: ancMid, PreviousTransactionID: ""},
			{TokenID: ancWhole, PreviousTransactionID: ""},
		},
	}

	getTxByIDReturning := func(info *models.TransactionInfo) func(string) (*models.TransactionInfo, error) {
		return func(id string) (*models.TransactionInfo, error) {
			if id == genTX {
				return info, nil
			}
			return nil, errors.New("not found")
		}
	}
	// burnMap: ancestorID -> (burnTx, burnt?). Missing entry => not burnt.
	burnFrom := func(m map[string]string) func(string) (string, bool, error) {
		return func(id string) (string, bool, error) {
			if tx, ok := m[id]; ok {
				return tx, true, nil
			}
			return "", false, nil
		}
	}

	t.Run("legit split: ancestors burnt by this genesis / free -> accept", func(t *testing.T) {
		// Immediate parent burnt by genTX (this split), higher ancestors free.
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfo),
			burnFrom(map[string]string{ancImmediate: genTX}),
			func([]string) (map[string]string, error) { return map[string]string{}, nil },
		)
		if err != nil {
			t.Fatalf("expected accept, got: %v", err)
		}
	})

	t.Run("DEEP replay: an ANCESTOR burnt by a different tx (empty-prev evasion) -> reject", func(t *testing.T) {
		// This is the case the old committed-parents check MISSED: the immediate
		// parent looks freshly minted (burnt by genTX), but a higher ancestor
		// (the resurrected level) is already burnt by an earlier split.
		burns := map[string]string{
			ancImmediate: genTX,   // freshly minted by this split
			ancMid:       genTX,   // part of this split's run
			ancWhole:     otherTX, // ALREADY consumed by an earlier split -> replay
		}
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfo),
			burnFrom(burns),
			func([]string) (map[string]string, error) {
				t.Fatal("Tier-1 should reject before authoritative")
				return nil, nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "double spend") {
			t.Fatalf("expected double spend rejection, got: %v", err)
		}
	})

	t.Run("legit re-split of a low part: higher ancestor burnt by another tx but NOT in this genesis -> accept", func(t *testing.T) {
		// The over-rejection guard. genTX splits only the immediate parent; the
		// whole token was legitimately burnt long ago by a different split. Since
		// genTX does NOT list the whole token in CommittedTokens, it must NOT be
		// checked. genInfoLow lists only the immediate parent.
		genInfoLow := &models.TransactionInfo{
			CommittedTokens: []*models.TokenInfo{
				{TokenID: ancImmediate, PreviousTransactionID: ""},
			},
		}
		burns := map[string]string{
			ancImmediate: genTX,   // this split burns it
			ancWhole:     otherTX, // legitimately burnt by an EARLIER split; genTX doesn't touch it
		}
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfoLow),
			burnFrom(burns),
			func([]string) (map[string]string, error) { return map[string]string{}, nil },
		)
		if err != nil {
			t.Fatalf("expected accept (higher ancestor not consumed by this split), got: %v", err)
		}
	})

	t.Run("replay caught only after authoritative sync -> reject", func(t *testing.T) {
		// Local view: ancestors not burnt (fresh quorum). Tier-2 sync returns
		// the authoritative burn map revealing the whole token already burnt by
		// otherTX. getParentBurnTx (tier-1) never resolves it locally.
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfo),
			burnFrom(map[string]string{}), // tier-1 finds nothing locally
			func(ids []string) (map[string]string, error) {
				return map[string]string{ancWhole: otherTX}, nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "double spend") {
			t.Fatalf("expected double spend after authoritative sync, got: %v", err)
		}
	})

	t.Run("legit first split confirmed after authoritative sync (all free) -> accept", func(t *testing.T) {
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfo),
			burnFrom(map[string]string{}), // nothing burnt locally
			// Authoritative sync serves the chains but none are burnt ("" per token).
			func(ids []string) (map[string]string, error) {
				m := make(map[string]string, len(ids))
				for _, id := range ids {
					m[id] = ""
				}
				return m, nil
			},
		)
		if err != nil {
			t.Fatalf("expected accept, got: %v", err)
		}
	})

	t.Run("fullnode unreachable -> fail open (no reject)", func(t *testing.T) {
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(genInfo),
			burnFrom(map[string]string{}), // never confirmable locally
			func([]string) (map[string]string, error) { return nil, errors.New("fullnode down") },
		)
		if err != nil {
			t.Fatalf("expected fail-open (nil), got: %v", err)
		}
	})

	t.Run("prev is not a split genesis -> pass through", func(t *testing.T) {
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			func(string) (*models.TransactionInfo, error) { return nil, errors.New("not found") },
			func(string) (string, bool, error) { t.Fatal("no ancestors to check"); return "", false, nil },
			func([]string) (map[string]string, error) { t.Fatal("no ancestors to sync"); return nil, nil },
		)
		if err != nil {
			t.Fatalf("expected pass-through, got: %v", err)
		}
	})

	t.Run("split genesis with empty committedTokens -> pass through", func(t *testing.T) {
		// getTxByID resolves but the tx has no committedTokens → not a split.
		plainGen := &models.TransactionInfo{}
		err := ValidateSplitParentsAgainstFullnode(
			transferInfo(), testLogger(),
			getTxByIDReturning(plainGen),
			func(string) (string, bool, error) { t.Fatal("no ancestors to check"); return "", false, nil },
			func([]string) (map[string]string, error) { t.Fatal("nothing to sync"); return nil, nil },
		)
		if err != nil {
			t.Fatalf("expected pass-through, got: %v", err)
		}
	})

	t.Run("whole-token transfer (no ancestors) -> pass through", func(t *testing.T) {
		wholeXfer := &models.TransactionInfo{
			Tokens: &models.TransactionTokens{
				RBT: []*models.TokenInfo{{TokenID: "1_1", PreviousTransactionID: genTX}},
			},
		}
		err := ValidateSplitParentsAgainstFullnode(
			wholeXfer, testLogger(),
			getTxByIDReturning(genInfo),
			func(string) (string, bool, error) { return "", false, nil },
			func([]string) (map[string]string, error) { return map[string]string{}, nil },
		)
		if err != nil {
			t.Fatalf("expected pass-through for whole token, got: %v", err)
		}
	})
}

// =============================================================================
// Fullnode-side (DB-dependent) checks.
//
// These exercise code paths that previously required a real *wallet.Wallet.
// With the new TxnStore interface in checks.go, we can drive them with a
// fully in-memory fake: fakeTxnStore.
// =============================================================================

// fakeTxnStore is an in-memory implementation of consensus.TxnStore.
// Every method is backed by a pluggable closure so each test row can shape
// just the calls it cares about. Any unset closure panics loudly — that is
// intentional: a panic in a test means the check reached a path you did not
// expect to be reached. Use the With... helpers below to set only what you
// need.
type fakeTxnStore struct {
	onGetTransactionByID                    func(id string, isFullNode bool) (*models.Transactions, error)
	onGetFullNodeRBTToken                   func(tokenID string) (models.FullNodeRBT, error)
	onGetLatestTransactionIdByTokenId       func(tokenID string, isFullNode bool) (string, error)
	onGetFullNodeTransactionAndRoleAtHeight func(tokenID string, height int64) (*models.Transactions, int16, error)
	onGetTransactionAndRoleAtHeight         func(tokenID string, height int64) (*models.Transactions, int16, error)
	onGetGenesisTransactionIdByTokenId      func(tokenID string, isFullNode bool) (string, error)
}

func (f *fakeTxnStore) GetTransactionByID(id string, isFullNode bool) (*models.Transactions, error) {
	if f.onGetTransactionByID == nil {
		panic("fakeTxnStore.GetTransactionByID called but not set up")
	}
	return f.onGetTransactionByID(id, isFullNode)
}
func (f *fakeTxnStore) GetFullNodeRBTToken(tokenID string) (models.FullNodeRBT, error) {
	if f.onGetFullNodeRBTToken == nil {
		panic("fakeTxnStore.GetFullNodeRBTToken called but not set up")
	}
	return f.onGetFullNodeRBTToken(tokenID)
}
func (f *fakeTxnStore) GetLatestTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	if f.onGetLatestTransactionIdByTokenId == nil {
		panic("fakeTxnStore.GetLatestTransactionIdByTokenId called but not set up")
	}
	return f.onGetLatestTransactionIdByTokenId(tokenID, isFullNode)
}
func (f *fakeTxnStore) GetFullNodeTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	if f.onGetFullNodeTransactionAndRoleAtHeight == nil {
		panic("fakeTxnStore.GetFullNodeTransactionAndRoleAtHeight called but not set up")
	}
	return f.onGetFullNodeTransactionAndRoleAtHeight(tokenID, height)
}
func (f *fakeTxnStore) GetTransactionAndRoleAtHeight(tokenID string, height int64) (*models.Transactions, int16, error) {
	if f.onGetTransactionAndRoleAtHeight == nil {
		panic("fakeTxnStore.GetTransactionAndRoleAtHeight called but not set up")
	}
	return f.onGetTransactionAndRoleAtHeight(tokenID, height)
}
func (f *fakeTxnStore) GetGenesisTransactionIdByTokenId(tokenID string, isFullNode bool) (string, error) {
	if f.onGetGenesisTransactionIdByTokenId == nil {
		panic("fakeTxnStore.GetGenesisTransactionIdByTokenId called but not set up")
	}
	return f.onGetGenesisTransactionIdByTokenId(tokenID, isFullNode)
}

// testLogger returns a real logger suitable for use inside tests. It writes
// nowhere meaningful (go test captures stdout), which is fine for our needs.
func testLogger() logger.Logger { return logger.New(nil) }

// =============================================================================
// Section 3 — TokenID-related checks for RBTs (ValidateNewTokenContent)
//
// Covers:
//   a. Token level being wrong
//   b. Token number being wrong
//   c. Part token index being wrong
//
// Uses all three network flavours (mainnet / testnet / localnet) so the
// offset math is exercised too.
// =============================================================================

func TestValidateNewTokenContent_RBT(t *testing.T) {
	log := testLogger()

	// MaxPossiblePartTokenNumber for MaxSupportedDecimalPlaces=3 is 1332 — see
	// core/parts/parts.go MaxPossiblePartsIndexByMaxDecimalPlaces. Use 2000 to
	// force an out-of-range part index.
	const outOfRangePartIndex = "2000"

	tests := []struct {
		name     string
		tokenID  string
		testnet  bool
		mainnet  bool
		localnet bool
		wantErr  bool
		errMatch string
	}{
		// ---- HAPPY PATHS ----
		{name: "mainnet valid whole token", tokenID: "1_1", mainnet: true, wantErr: false},
		{name: "testnet valid whole token", tokenID: "50001_1", testnet: true, wantErr: false},
		{name: "localnet valid whole token", tokenID: "10001_1", localnet: true, wantErr: false},
		{name: "mainnet valid part token", tokenID: "1_1_1", mainnet: true, wantErr: false},

		// ---- 3.a  Token level being wrong ----
		{name: "non-numeric level", tokenID: "abc_1", mainnet: true, wantErr: true, errMatch: "invalid token level"},
		{name: "mainnet level out of TokenMap", tokenID: "99999_1", mainnet: true, wantErr: true, errMatch: "not present in TokenMap"},
		{name: "testnet level below offset (50000)", tokenID: "100_1", testnet: true, wantErr: true, errMatch: "testnet level must be >="},
		{name: "localnet level below offset (10000)", tokenID: "500_1", localnet: true, wantErr: true, errMatch: "localnet level must be >"},

		// ---- 3.b  Token number being wrong ----
		{name: "non-numeric token number", tokenID: "1_abc", mainnet: true, wantErr: true, errMatch: "invalid token number"},
		{name: "token number exceeds max for level 1 (4,300,000)", tokenID: "1_4300001", mainnet: true, wantErr: true, errMatch: "exceeds max allowed"},
		{name: "negative token number", tokenID: "1_-5", mainnet: true, wantErr: true, errMatch: "exceeds max allowed"}, // negative passes strconv but fails maxAllowed guard (tokenNo<0)

		// ---- 3.c  Part token index being wrong ----
		{name: "non-numeric part index", tokenID: "1_1_abc", mainnet: true, wantErr: true, errMatch: "invalid part number"},
		{name: "part index above max (1332)", tokenID: "1_1_" + outOfRangePartIndex, mainnet: true, wantErr: true, errMatch: "exceeds max allowed"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewTokenContent(tc.tokenID, false, tc.testnet, tc.mainnet, tc.localnet, log)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errMatch)
			}
		})
	}
}
