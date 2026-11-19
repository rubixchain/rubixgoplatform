package core

import (
	"strings"
	"testing"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

func TestCheckRecoveryRequestTiming(t *testing.T) {
	deploy := int16(models.GetTokenRoleID(constants.TokenRole_Deploy))
	execute := int16(models.GetTokenRoleID(constants.TokenRole_Execute))
	tx := func(id, did string, role int16, epoch int) types.RecoveryTx {
		return types.RecoveryTx{ID: id, Role: role, Info: models.TransactionInfo{Initiator: did, Epoch: epoch}}
	}
	const now = 1_800_000_000
	const day = 24 * 60 * 60

	tests := []struct {
		name    string
		chain   []types.RecoveryTx
		wantErr string
	}{
		{"first request", []types.RecoveryTx{tx("d", "deployer", deploy, now-3*day), tx("r", "alice", execute, now)}, ""},
		{"older than 2 minutes", []types.RecoveryTx{tx("r", "alice", execute, now-121)}, "older than"},
		{"same DID within 48 hours", []types.RecoveryTx{tx("p", "alice", execute, now-47*60*60), tx("r", "alice", execute, now)}, "48 hours"},
		{"same DID after 48 hours", []types.RecoveryTx{tx("p", "alice", execute, now-2*day), tx("r", "alice", execute, now)}, ""},
		{"other DID within 48 hours", []types.RecoveryTx{tx("p", "bob", execute, now-60), tx("r", "alice", execute, now)}, ""},
		{"user deployed the Recovery NFT", []types.RecoveryTx{tx("d", "alice", deploy, now-60), tx("r", "alice", execute, now)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRecoveryRequestTiming(tt.chain, len(tt.chain)-1, now)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("got %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRecoveryMessage(t *testing.T) {
	msg := func(typ, sender string) string {
		return `{"type":"` + typ + `","body":{},"sender_did":"` + sender + `"}`
	}
	tests := []struct {
		name      string
		data      string
		initiator string
		ok        bool
	}{
		{"commitment from a quorum", msg("commitment", "q1"), "q1", true},
		{"unpledged from anyone", msg("unpledged", "x"), "x", true},
		{"challenge from anyone", msg("challenge", "x"), "x", true},
		{"completion from the user", msg("completion", "user"), "user", true},
		{"mismatch from the user", msg("mismatch", "user"), "user", true},
		{"completion from another DID", msg("completion", "q1"), "q1", false},
		{"mismatch from another DID", msg("mismatch", "q1"), "q1", false},
		{"sender is not the initiator", msg("commitment", "q2"), "q1", false},
		{"unknown type", msg("other", "q1"), "q1", false},
		{"not JSON", "x", "q1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRecoveryMessage(tt.data, tt.initiator, "user")
			if (err == nil) != tt.ok {
				t.Fatalf("got err %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestRecoveryDeadline(t *testing.T) {
	const epoch = 1_800_000_000
	if got := recoveryDeadline(epoch, constants.RecoveryState_SyncOpen); got != epoch+10*60+60 {
		t.Fatalf("commit deadline: got %d", got)
	}
	if got := recoveryDeadline(epoch, constants.RecoveryState_SyncClosed); got != epoch+40*60+60 {
		t.Fatalf("sync deadline: got %d", got)
	}
}
