package recoverycommit

import (
	"encoding/json"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func roleID(name string) int16 { return int16(models.GetTokenRoleID(name)) }

func infoJSON(t *testing.T, info models.TransactionInfo) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func tokens(ids ...string) []*models.TokenInfo {
	out := make([]*models.TokenInfo, len(ids))
	for i, id := range ids {
		out[i] = &models.TokenInfo{TokenID: id}
	}
	return out
}

// chainOf links infos into a valid chain with the given roles.
func chainOf(t *testing.T, roles []string, infos ...models.TransactionInfo) []ChainEntry {
	t.Helper()
	chain := make([]ChainEntry, len(infos))
	for i, info := range infos {
		e := ChainEntry{TransactionID: "tx" + string(rune('0'+i)), Position: int64(i), Role: roleID(roles[i]), Info: infoJSON(t, info)}
		if i > 0 {
			e.PreviousTransactionID = chain[i-1].TransactionID
		}
		chain[i] = e
	}
	return chain
}

func TestValidateChain(t *testing.T) {
	info := models.TransactionInfo{Initiator: "a"}
	good := chainOf(t, []string{constants.TokenRole_Mint, constants.TokenRole_Transfer, constants.TokenRole_Transfer}, info, info, info)
	if err := ValidateChain(good); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}

	mutate := func(f func(c []ChainEntry) []ChainEntry) []ChainEntry {
		c := append([]ChainEntry(nil), good...)
		return f(c)
	}
	bad := map[string][]ChainEntry{
		"empty":             nil,
		"missing genesis":   good[1:],
		"position gap":      mutate(func(c []ChainEntry) []ChainEntry { return append(c[:1], c[2:]...) }),
		"broken link":       mutate(func(c []ChainEntry) []ChainEntry { c[2].PreviousTransactionID = "other"; return c }),
		"genesis with prev": mutate(func(c []ChainEntry) []ChainEntry { c[0].PreviousTransactionID = "x"; return c }),
		"missing tx info":   mutate(func(c []ChainEntry) []ChainEntry { c[1].Info = nil; return c }),
		"empty tx ID":       mutate(func(c []ChainEntry) []ChainEntry { c[1].TransactionID = ""; return c }),
		"repeated tx": mutate(func(c []ChainEntry) []ChainEntry {
			c[2].TransactionID = c[0].TransactionID
			return c
		}),
	}
	for name, chain := range bad {
		if err := ValidateChain(chain); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestCurrentOwner(t *testing.T) {
	const tok = "T1"
	mint := models.TransactionInfo{Initiator: "alice", Owner: "alice", Tokens: &models.TransactionTokens{RBT: tokens(tok)}}
	toBob := models.TransactionInfo{Initiator: "alice", Owner: "bob", Tokens: &models.TransactionTokens{RBT: tokens(tok)}}
	R := func(names ...string) []string { return names }
	mintR, xfer := constants.TokenRole_Mint, constants.TokenRole_Transfer

	cases := []struct {
		name  string
		chain []ChainEntry
		want  string
	}{
		{"genesis owner", chainOf(t, R(mintR), mint), "alice"},
		{"latest transfer wins over history", chainOf(t, R(mintR, xfer), mint, toBob), "bob"},
		{"owner falls back to initiator",
			chainOf(t, R(mintR), models.TransactionInfo{Initiator: "alice", Tokens: &models.TransactionTokens{FT: tokens(tok)}}), "alice"},
		{"genesis not naming the token",
			chainOf(t, R(mintR), models.TransactionInfo{Initiator: "alice", Owner: "carol"}), "carol"},
		{"burnt token has no owner", chainOf(t, R(mintR, constants.TokenRole_Burn), mint, toBob), ""},
		{"committed role has no owner", chainOf(t, R(mintR, constants.TokenRole_Commit), mint, toBob), ""},
		{"committed tokens list has no owner",
			chainOf(t, R(mintR, xfer), mint, models.TransactionInfo{Initiator: "alice", Owner: "bob", CommittedTokens: tokens(tok)}), ""},
		{"pledge belongs to the quorum",
			chainOf(t, R(mintR, constants.TokenRole_Pledge), mint,
				models.TransactionInfo{Initiator: "x", Owner: "y", Quorums: []*models.QuorumInfo{{Did: "q1", Tokens: tokens("other")}, {Did: "q2", Tokens: tokens(tok)}}}), "q2"},
		{"unpledge proof tx inherits from pledge",
			chainOf(t, R(mintR, constants.TokenRole_Pledge, constants.TokenRole_Unpledge), mint,
				models.TransactionInfo{Initiator: "x", Owner: "y", Quorums: []*models.QuorumInfo{{Did: "q2", Tokens: tokens(tok)}}},
				models.TransactionInfo{Initiator: "z", Owner: "w", Tokens: &models.TransactionTokens{RBT: tokens("unrelated")}}), "q2"},
		{"nft deploy", chainOf(t, R(constants.TokenRole_Deploy),
			models.TransactionInfo{Initiator: "alice", Owner: "alice", Tokens: &models.TransactionTokens{NFT: tokens(tok)}}), "alice"},
		{"single nft transaction", chainOf(t, R(constants.TokenRole_Deploy, xfer),
			models.TransactionInfo{Initiator: "alice", Tokens: &models.TransactionTokens{NFT: tokens(tok)}},
			models.TransactionInfo{Initiator: "alice", Owner: "bob", Tokens: &models.TransactionTokens{NFT: tokens(tok)}}), "bob"},
		{"nft in mixed transaction is ambiguous", chainOf(t, R(constants.TokenRole_Deploy, xfer),
			models.TransactionInfo{Initiator: "alice", Tokens: &models.TransactionTokens{NFT: tokens(tok)}},
			models.TransactionInfo{Initiator: "alice", Owner: "bob", Tokens: &models.TransactionTokens{NFT: tokens(tok), RBT: tokens("r")}}), ""},
		{"smart contract execute", chainOf(t, R(constants.TokenRole_Deploy, constants.TokenRole_Execute),
			models.TransactionInfo{Initiator: "alice", Tokens: &models.TransactionTokens{SmartContract: tokens(tok)}},
			models.TransactionInfo{Initiator: "bob", Tokens: &models.TransactionTokens{SmartContract: tokens(tok)}}), "bob"},
	}
	for _, c := range cases {
		got, err := CurrentOwner(tok, c.chain)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: owner = %q, want %q", c.name, got, c.want)
		}
	}

	bad := chainOf(t, R(mintR), mint)
	bad[0].Info = json.RawMessage("{")
	if _, err := CurrentOwner(tok, bad); err == nil {
		t.Fatal("expected decode error")
	}
}
