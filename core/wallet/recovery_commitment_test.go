package wallet

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// newPGTestWallet opens a wallet on the throwaway database named by
// RUBIX_TEST_PG_DSN (e.g. "postgres://u:p@127.0.0.1:5432/db"), or skips.
func newPGTestWallet(t *testing.T) *Wallet {
	t.Helper()
	dsn := os.Getenv("RUBIX_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("RUBIX_TEST_PG_DSN not set")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := storage.NewRubixDB(ctx, &types.DBConfig{
		Host: cfg.Host, Port: int(cfg.Port), Username: cfg.User, Password: cfg.Password, DBName: cfg.Database,
	}, storage.DBOpts{MaxConns: 2, MinConns: 1, MaxConnLifetimeInSeconds: time.Minute, MaxConnIdleTimeInSeconds: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	w, err := NewWallet(ctx, db, logger.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestGetRecoveryPledgedTokens(t *testing.T) {
	w := newPGTestWallet(t)
	ns := fmt.Sprintf("rc%d-", time.Now().UnixNano())
	user, q, q2 := ns+"user", ns+"q", ns+"q2"
	id := func(s string) string { return ns + s }

	addTx := func(txID, owner string) {
		info, err := models.SerializeTransactionInfo(&models.TransactionInfo{Initiator: "x", Owner: owner})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.db.Pool().Exec(w.Ctx, `INSERT INTO transactions (id, info, signature) VALUES ($1, $2, '{}')`, txID, info); err != nil {
			t.Fatal(err)
		}
	}
	addPledge := func(txID, quorum string, tokens ...string) {
		if _, err := w.db.Pool().Exec(w.Ctx, `
			INSERT INTO unpledge_sequence_info (tx_id, pledge_tokens, epoch, quorum_did, transaction_tokens)
			VALUES ($1, $2, 0, $3, $4)`, txID, []string{txID + "-pledge"}, quorum, tokens); err != nil {
			t.Fatal(err)
		}
	}

	addTx(id("x0"), user) // older live pledge for T1 (re-received)
	addPledge(id("x0"), q, id("T1"))
	addTx(id("x1"), user) // live pledge for T1 and T2
	addPledge(id("x1"), q, id("T1"), id("T2"))
	addTx(id("x2"), user) // other quorum
	addPledge(id("x2"), q2, id("T3"))
	addTx(id("x3"), "bob") // other receiver
	addPledge(id("x3"), q, id("T4"))
	addTx(id("x4"), user) // released: unpledge deletes the whole row
	addPledge(id("x4"), q, id("T5"), id("T6"))
	if _, err := w.db.Pool().Exec(w.Ctx, `DELETE FROM unpledge_sequence_info WHERE tx_id = $1`, id("x4")); err != nil {
		t.Fatal(err)
	}
	addPledge(id("x9"), q, id("T9")) // transaction row missing: cannot be attributed

	got, err := w.GetRecoveryPledgedTokens(q, user)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		id("T1"): {id("x0"), id("x1")},
		id("T2"): {id("x1")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pledged = %v, want %v", got, want)
	}
}

func TestGetRecoveryTokenChains(t *testing.T) {
	w := newPGTestWallet(t)
	ns := fmt.Sprintf("rc%d-", time.Now().UnixNano())
	id := func(s string) string { return ns + s }
	mint := int16(models.GetTokenRoleID(constants.TokenRole_Mint))
	xfer := int16(models.GetTokenRoleID(constants.TokenRole_Transfer))

	for _, tx := range []string{id("m1"), id("x1")} {
		if _, err := w.db.Pool().Exec(w.Ctx, `INSERT INTO transactions (id, info, signature) VALUES ($1, '{"owner":"u"}', '{}')`, tx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.db.Pool().Exec(w.Ctx, `INSERT INTO dids (did) VALUES ($1)`, id("u")); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{id("T1"), id("T3")} {
		if _, err := w.db.Pool().Exec(w.Ctx, `
			INSERT INTO tokens (token_id, token_value, did, transaction_id, token_state_hash, token_type)
			VALUES ($1, 1, $2, $3, '', 1)`, tok, id("u"), id("m1")); err != nil {
			t.Fatal(err)
		}
	}
	addLink := func(token, tx string, prev *string, role int16, pos int64) {
		if _, err := w.db.Pool().Exec(w.Ctx, `
			INSERT INTO tokenchain (token_id, transaction_id, previous_transaction_id, role, position)
			VALUES ($1, $2, $3, $4, $5)`, token, tx, prev, role, pos); err != nil {
			t.Fatal(err)
		}
	}
	m1 := id("m1")
	addLink(id("T1"), id("x1"), &m1, xfer, 1) // inserted out of order
	addLink(id("T1"), id("m1"), nil, mint, 0)
	addLink(id("T3"), id("m1"), nil, mint, 0) // not requested

	got, err := w.GetRecoveryTokenChains([]string{id("T1"), id("T4")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got chains for %d tokens, want 1 (T1)", len(got))
	}
	t1 := got[id("T1")]
	if len(t1) != 2 || t1[0].TransactionID != id("m1") || t1[1].TransactionID != id("x1") ||
		t1[1].PreviousTransactionID != id("m1") || t1[1].Role != xfer || len(t1[1].Info) == 0 {
		t.Fatalf("T1 chain = %+v", t1)
	}
}
