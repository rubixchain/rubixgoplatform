package core

import (
	"reflect"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// tok is a terse constructor for the (tokenID, previousTransactionID) pairs that
// these tests are made of.
func tok(tokenID, prevTxID string) *models.TokenInfo {
	return &models.TokenInfo{TokenID: tokenID, PreviousTransactionID: prevTxID}
}

func TestTransactionDependencies(t *testing.T) {
	tests := []struct {
		name string
		info *models.TransactionInfo
		want []string
	}{
		{
			name: "nil info",
			info: nil,
			want: nil,
		},
		{
			name: "nil Tokens",
			info: &models.TransactionInfo{},
			want: nil,
		},
		{
			name: "empty Tokens",
			info: &models.TransactionInfo{Tokens: &models.TransactionTokens{}},
			want: nil,
		},
		{
			name: "genesis only - every prev empty",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("child-1", ""), tok("child-2", "")},
				},
			},
			want: nil,
		},
		{
			name: "single RBT with prev",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("tok-X", "txn-S")},
				},
			},
			want: []string{"txn-S"},
		},
		{
			name: "committed tokens contribute dependencies",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("child", "")},
				},
				CommittedTokens: []*models.TokenInfo{tok("parent", "txn-P")},
			},
			want: []string{"txn-P"},
		},
		{
			// The split leg of a transfer: new children at genesis, burnt parent
			// carrying the only real dependency.
			name: "split shape - genesis children plus burnt parent",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("child-1", ""), tok("child-2", "")},
				},
				CommittedTokens: []*models.TokenInfo{tok("parent", "txn-parent-prev")},
			},
			want: []string{"txn-parent-prev"},
		},
		{
			name: "multi-quorum pledge tokens are included",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("tok-X", "txn-S")},
				},
				Quorums: []*models.QuorumInfo{
					{Did: "did-q1", Tokens: []*models.TokenInfo{tok("pledge-1", "txn-Q1")}},
					{Did: "did-q2", Tokens: []*models.TokenInfo{tok("pledge-2", "txn-Q2")}},
				},
			},
			want: []string{"txn-S", "txn-Q1", "txn-Q2"},
		},
		{
			name: "duplicate prev IDs across lists are deduped, first occurrence wins",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("tok-A", "txn-S"), tok("tok-B", "txn-S")},
					FT:  []*models.TokenInfo{tok("tok-C", "txn-S")},
				},
				CommittedTokens: []*models.TokenInfo{tok("tok-D", "txn-S")},
				Quorums: []*models.QuorumInfo{
					{Did: "did-q1", Tokens: []*models.TokenInfo{tok("pledge-1", "txn-S")}},
				},
			},
			want: []string{"txn-S"},
		},
		{
			name: "canonical order - RBT, FT, NFT, SmartContract, Committed, Quorums",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT:           []*models.TokenInfo{tok("t-rbt", "prev-rbt")},
					FT:            []*models.TokenInfo{tok("t-ft", "prev-ft")},
					NFT:           []*models.TokenInfo{tok("t-nft", "prev-nft")},
					SmartContract: []*models.TokenInfo{tok("t-sc", "prev-sc")},
				},
				CommittedTokens: []*models.TokenInfo{tok("t-committed", "prev-committed")},
				Quorums: []*models.QuorumInfo{
					{Did: "did-q1", Tokens: []*models.TokenInfo{tok("t-pledge", "prev-pledge")}},
				},
			},
			want: []string{
				"prev-rbt", "prev-ft", "prev-nft", "prev-sc",
				"prev-committed", "prev-pledge",
			},
		},
		{
			name: "nil element pointers are skipped, not dereferenced",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{nil, tok("tok-X", "txn-S"), nil},
				},
				CommittedTokens: []*models.TokenInfo{nil},
				Quorums: []*models.QuorumInfo{
					nil,
					{Did: "did-q1", Tokens: []*models.TokenInfo{nil, tok("pledge-1", "txn-Q1")}},
				},
			},
			want: []string{"txn-S", "txn-Q1"},
		},
		{
			// CommittedTokens live outside the `Tokens != nil` guard here, unlike
			// wallet.collectFullNodeTokenInputs. See forEachTokenInfo's doc comment.
			name: "committed tokens are found even when Tokens is nil",
			info: &models.TransactionInfo{
				CommittedTokens: []*models.TokenInfo{tok("parent", "txn-P")},
			},
			want: []string{"txn-P"},
		},
		{
			name: "quorum tokens are found even when Tokens is nil",
			info: &models.TransactionInfo{
				Quorums: []*models.QuorumInfo{
					{Did: "did-q1", Tokens: []*models.TokenInfo{tok("pledge-1", "txn-Q1")}},
				},
			},
			want: []string{"txn-Q1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := transactionDependencies(tc.info)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("transactionDependencies() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTransactionTokenIDs(t *testing.T) {
	tests := []struct {
		name string
		info *models.TransactionInfo
		want []string
	}{
		{
			name: "nil info",
			info: nil,
			want: nil,
		},
		{
			name: "nil Tokens",
			info: &models.TransactionInfo{},
			want: nil,
		},
		{
			// The key difference from transactionDependencies: a genesis token has
			// no dependency but is still a token this transaction affects.
			name: "genesis tokens are included",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("child-1", ""), tok("child-2", "")},
				},
			},
			want: []string{"child-1", "child-2"},
		},
		{
			name: "canonical order across every list",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT:           []*models.TokenInfo{tok("t-rbt", "p")},
					FT:            []*models.TokenInfo{tok("t-ft", "p")},
					NFT:           []*models.TokenInfo{tok("t-nft", "p")},
					SmartContract: []*models.TokenInfo{tok("t-sc", "p")},
				},
				CommittedTokens: []*models.TokenInfo{tok("t-committed", "p")},
				Quorums: []*models.QuorumInfo{
					{Did: "did-q1", Tokens: []*models.TokenInfo{tok("t-pledge", "p")}},
				},
			},
			want: []string{"t-rbt", "t-ft", "t-nft", "t-sc", "t-committed", "t-pledge"},
		},
		{
			name: "duplicate token IDs are deduped",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("tok-X", "prev-1"), tok("tok-X", "prev-2")},
				},
			},
			want: []string{"tok-X"},
		},
		{
			name: "empty token IDs are skipped",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{tok("", "prev-1"), tok("tok-X", "prev-2")},
				},
			},
			want: []string{"tok-X"},
		},
		{
			name: "nil element pointers are skipped",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{nil, tok("tok-X", "")},
				},
				Quorums: []*models.QuorumInfo{nil},
			},
			want: []string{"tok-X"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := transactionTokenIDs(tc.info)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("transactionTokenIDs() = %v, want %v", got, tc.want)
			}
		})
	}
}

// bigTxnInfo is a transaction wide enough that a map-ordered traversal would
// almost certainly reorder it between runs.
func bigTxnInfo() *models.TransactionInfo {
	info := &models.TransactionInfo{
		Tokens:          &models.TransactionTokens{},
		CommittedTokens: []*models.TokenInfo{},
	}
	for i := 0; i < 8; i++ {
		s := string(rune('a' + i))
		info.Tokens.RBT = append(info.Tokens.RBT, tok("rbt-"+s, "prev-rbt-"+s))
		info.Tokens.FT = append(info.Tokens.FT, tok("ft-"+s, "prev-ft-"+s))
		info.Tokens.NFT = append(info.Tokens.NFT, tok("nft-"+s, "prev-nft-"+s))
		info.Tokens.SmartContract = append(info.Tokens.SmartContract, tok("sc-"+s, "prev-sc-"+s))
		info.CommittedTokens = append(info.CommittedTokens, tok("com-"+s, "prev-com-"+s))
		info.Quorums = append(info.Quorums, &models.QuorumInfo{
			Did:    "did-" + s,
			Tokens: []*models.TokenInfo{tok("pledge-"+s, "prev-pledge-"+s)},
		})
	}
	return info
}

// Both helpers feed decisions that must be reproducible across workers and
// across retries of the same transaction, so ordering must not vary run to run.
// consensus.TokenChainIntegrityCheck ranges over a map and does vary; this is
// the guard that these helpers do not.
func TestTraversalIsDeterministic(t *testing.T) {
	info := bigTxnInfo()

	wantDeps := transactionDependencies(info)
	wantTokens := transactionTokenIDs(info)

	if len(wantDeps) != 48 {
		t.Fatalf("fixture produced %d dependencies, expected 48", len(wantDeps))
	}
	if len(wantTokens) != 48 {
		t.Fatalf("fixture produced %d token IDs, expected 48", len(wantTokens))
	}

	for i := 0; i < 100; i++ {
		if got := transactionDependencies(info); !reflect.DeepEqual(got, wantDeps) {
			t.Fatalf("transactionDependencies() varied on run %d:\n got %v\nwant %v", i, got, wantDeps)
		}
		if got := transactionTokenIDs(info); !reflect.DeepEqual(got, wantTokens) {
			t.Fatalf("transactionTokenIDs() varied on run %d:\n got %v\nwant %v", i, got, wantTokens)
		}
	}
}

// The helpers must not mutate the transaction they are handed — they run on the
// pubsub ingest path, where the same event is passed to validation afterwards.
func TestTraversalDoesNotMutateInput(t *testing.T) {
	info := bigTxnInfo()
	before := reflect.DeepEqual(info, bigTxnInfo())
	if !before {
		t.Fatal("fixture is not reproducible; test cannot detect mutation")
	}

	transactionDependencies(info)
	transactionTokenIDs(info)

	if !reflect.DeepEqual(info, bigTxnInfo()) {
		t.Error("transaction info was mutated by the traversal helpers")
	}
}
