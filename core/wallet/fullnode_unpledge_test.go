package wallet

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

type unpledgeTestRow []any

func (r unpledgeTestRow) Scan(dest ...any) error {
	for i, value := range r {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

// Embed the unused pgx.Tx methods; unexpected calls fail the test.
type unpledgeTestTx struct {
	pgx.Tx
	info   []byte
	tip    string
	role   int16
	writes [][]any
}

func (tx *unpledgeTestTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "SELECT info"):
		return unpledgeTestRow{tx.info}
	case strings.Contains(sql, "FOR UPDATE"):
		return unpledgeTestRow{"collateral"}
	case strings.Contains(sql, "FROM fullnode_tokenchain"):
		previous := "genesis"
		return unpledgeTestRow{int32(1), "collateral", tx.tip, &previous, tx.role, int64(2), time.Time{}, time.Time{}}
	default:
		return unpledgeTestRow{"collateral", pgtype.Text{}, float64(1), int16(constants.TokenStatus_Pledged), "quorum", "previous", "hash", int64(2), tx.role}
	}
}

func (tx *unpledgeTestTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.writes = append(tx.writes, args)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestPersistFullNodeUnpledges(t *testing.T) {
	pledge := int16(models.GetTokenRoleID(constants.TokenRole_Pledge))
	unpledge := int16(models.GetTokenRoleID(constants.TokenRole_Unpledge))
	for _, tc := range []struct {
		name, tip, inputRole, token string
		role                        int16
		want                        bool
	}{
		{"live collateral", "previous", constants.TokenRole_Transfer, "moved", pledge, true},
		{"duplicate successor", "current", constants.TokenRole_Transfer, "moved", unpledge, false},
		{"released by another successor", "other", constants.TokenRole_Transfer, "moved", unpledge, false},
		{"pledged again", "new-pledge", constants.TokenRole_Transfer, "moved", pledge, false},
		{"new quorum collateral does not trigger release", "previous", constants.TokenRole_Pledge, "moved", pledge, false},
		{"unpledge proof is not a transfer", "previous", constants.TokenRole_Transfer, "collateral", pledge, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := json.Marshal(&models.TransactionInfo{
				Tokens:  &models.TransactionTokens{RBT: []*models.TokenInfo{{TokenID: "moved"}, {TokenID: "sibling"}}},
				Quorums: []*models.QuorumInfo{{Did: "quorum", Tokens: []*models.TokenInfo{{TokenID: "collateral"}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			tx := &unpledgeTestTx{info: info, tip: tc.tip, role: tc.role}
			// Moving one token releases the whole transaction's collateral.
			inputs := []fullNodeTokenInput{{tokenInfo: &models.TokenInfo{TokenID: tc.token, PreviousTransactionID: "previous"}, roleName: tc.inputRole}}
			ids, err := new(Wallet).persistFullNodeUnpledges(context.Background(), tx, "current", inputs)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.want {
				if len(ids) != 0 || len(tx.writes) != 0 {
					t.Fatalf("unexpected release: %v, writes %v", ids, tx.writes)
				}
				return
			}
			if !reflect.DeepEqual(ids, []string{"collateral"}) || len(tx.writes) != 2 {
				t.Fatalf("release missing: %v, writes %v", ids, tx.writes)
			}
			chain := tx.writes[0]
			if chain[1] != "current" || *chain[2].(*string) != "previous" || chain[3] != unpledge || chain[4] != int64(3) {
				t.Fatalf("wrong unpledge entry: %v", chain)
			}
			state := tx.writes[1]
			if state[2] != float64(1) || state[3] != int16(constants.TokenStatus_Free) || state[4] != "quorum" || state[5] != "current" || state[6] != "hash" || state[7] != int64(3) || state[8] != unpledge {
				t.Fatalf("wrong collateral state: %v", state)
			}
		})
	}
}
