package models_test

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

// Two syntactically valid 46-char v0 CIDs for fixtures.
const (
	validQmCID  = "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco"
	validQmCID2 = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
)

// Round-trip: a well-formed document survives serialize/parse unchanged.
func TestTokenPropertiesRoundTrip(t *testing.T) {
	in := &models.TokenProperties{
		Version: models.PropertiesDocVersion,
		Flags:   models.FlagTransferable,
		Policy:  models.PropertiesPolicy{ValidFrom: 1775001700, ValidTo: 1775002000},
		Restriction: models.PropertiesRestrict{
			Whitelist:             validQmCID,
			Admins:                validQmCID2,
			AllowedSubnets:        []string{"subnet-a"},
			AllowedSmartContracts: []string{"QmSomeContract"},
		},
	}

	raw, err := in.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	out, err := models.ParseTokenProperties(raw)
	if err != nil {
		t.Fatalf("ParseTokenProperties: %v", err)
	}
	if out.Version != in.Version || out.Flags != in.Flags {
		t.Errorf("version/flags not preserved: got v=%d f=%d", out.Version, out.Flags)
	}
	if out.Policy != in.Policy {
		t.Errorf("policy not preserved: got %+v want %+v", out.Policy, in.Policy)
	}
	if out.Restriction.Whitelist != in.Restriction.Whitelist ||
		out.Restriction.Admins != in.Restriction.Admins {
		t.Errorf("restriction CIDs not preserved: got %+v", out.Restriction)
	}
	if !out.IsTransferable() {
		t.Error("transferable flag lost in round trip")
	}
}

// Pins the compact wire keys so a field rename breaks visibly.
func TestTokenPropertiesWireKeys(t *testing.T) {
	doc := &models.TokenProperties{Version: models.PropertiesDocVersion}
	raw, err := doc.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	const want = `{"v":1,"f":0,"p":{"valid_from":0,"valid_to":0},"r":{"whitelist":"","admins":"","allowed_subnets":null,"allowed_smart_contracts":null}}`
	if string(raw) != want {
		t.Errorf("properties wire shape changed.\n want: %s\n  got: %s", want, string(raw))
	}
}

// Every case here must be REJECTED; any that starts passing is a fail-open bug.
func TestParseTokenPropertiesFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty input", ``},
		{"not json", `not json at all`},
		{"truncated json", `{"v":1,`},
		{"unrecognised future version", `{"v":2,"f":0,"p":{},"r":{}}`},
		{"version zero", `{"v":0,"f":0,"p":{},"r":{}}`},
		{"negative version", `{"v":-1,"f":0,"p":{},"r":{}}`},
		// The design doc's example literal: bits 1 and 7 are undefined at v1.
		{"team example f=131", `{"v":1,"f":131,"p":{},"r":{}}`},
		{"single unknown bit", `{"v":1,"f":2,"p":{},"r":{}}`},
		{"high unknown bit", `{"v":1,"f":128,"p":{},"r":{}}`},
		{"unknown top-level field", `{"v":1,"f":0,"p":{},"r":{},"extra":true}`},
		{"unknown nested field", `{"v":1,"f":0,"p":{"nope":1},"r":{}}`},
		{"negative valid_from", `{"v":1,"f":0,"p":{"valid_from":-5,"valid_to":0},"r":{}}`},
		{"year number instead of epoch", `{"v":1,"f":0,"p":{"valid_from":0,"valid_to":2027},"r":{}}`},
		{"valid_from below network start", `{"v":1,"f":0,"p":{"valid_from":1000000000,"valid_to":0},"r":{}}`},
		{"inverted window", `{"v":1,"f":0,"p":{"valid_from":1775002000,"valid_to":1775001700},"r":{}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := models.ParseTokenProperties([]byte(tc.raw))
			if err == nil {
				t.Fatalf("expected rejection, got a parsed document: %+v", got)
			}
			if got != nil {
				t.Errorf("rejected input must yield a nil document, got %+v", got)
			}
		})
	}
}

// The minimal document: everything absent means unrestricted, and must parse.
func TestParseTokenPropertiesMinimalIsUnrestricted(t *testing.T) {
	doc, err := models.ParseTokenProperties([]byte(`{"v":1,"f":0,"p":{},"r":{}}`))
	if err != nil {
		t.Fatalf("minimal document should parse: %v", err)
	}
	if doc.IsTransferable() {
		t.Error("transferable must be unset when f=0")
	}
	if !doc.IsWithinValidityWindow(0) || !doc.IsWithinValidityWindow(1<<40) {
		t.Error("a zero window must be unbounded on both sides")
	}
	if doc.Restriction.Whitelist != "" || doc.Restriction.Admins != "" {
		t.Error("absent CIDs should stay empty")
	}
}

func TestIsWithinValidityWindow(t *testing.T) {
	cases := []struct {
		name            string
		from, to, epoch int64
		want            bool
	}{
		{"unbounded", 0, 0, 1000, true},
		{"inside closed window", 100, 200, 150, true},
		{"on lower bound", 100, 200, 100, true},
		{"on upper bound", 100, 200, 200, true},
		{"before window", 100, 200, 99, false},
		{"after window", 100, 200, 201, false},
		{"lower bound only, after", 100, 0, 5000, true},
		{"lower bound only, before", 100, 0, 50, false},
		{"upper bound only, before", 0, 200, 50, true},
		{"upper bound only, after", 0, 200, 250, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &models.TokenProperties{
				Policy: models.PropertiesPolicy{ValidFrom: tc.from, ValidTo: tc.to},
			}
			if got := p.IsWithinValidityWindow(tc.epoch); got != tc.want {
				t.Errorf("epoch %d in [%d,%d]: got %v want %v", tc.epoch, tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestParsePropertiesEntries(t *testing.T) {
	got, err := models.ParsePropertiesEntries([]byte(`{"entries":["did-a","did-b"]}`))
	if err != nil {
		t.Fatalf("ParsePropertiesEntries: %v", err)
	}
	if len(got) != 2 || got[0] != "did-a" || got[1] != "did-b" {
		t.Errorf("unexpected entries: %v", got)
	}

	// An explicitly empty list is valid and means unrestricted.
	empty, err := models.ParsePropertiesEntries([]byte(`{"entries":[]}`))
	if err != nil {
		t.Fatalf("empty entries should parse: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no entries, got %v", empty)
	}

	bad := []string{
		``,
		`not json`,
		`{"entries":["ok",""]}`,      // blank entry
		`{"entries":["ok","  "]}`,    // whitespace-only entry
		`{"entries":["a"],"x":true}`, // unknown field
	}
	for _, raw := range bad {
		if _, err := models.ParsePropertiesEntries([]byte(raw)); err == nil {
			t.Errorf("expected rejection for %q", raw)
		}
	}
}
