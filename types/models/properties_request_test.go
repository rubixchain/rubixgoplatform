package models_test

import (
	"encoding/json"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
)

func TestPropertiesInfoToDocument(t *testing.T) {
	info := &models.PropertiesInfo{
		Transferable:          true,
		ValidFrom:             1775001700,
		ValidTo:               1775002000,
		AllowedSubnets:        []string{"testnet"},
		AllowedSmartContracts: []string{"QmContract"},
	}
	doc := info.ToDocument()

	if doc.Version != models.PropertiesDocVersion {
		t.Errorf("version should be %d, got %d", models.PropertiesDocVersion, doc.Version)
	}
	if !doc.IsTransferable() {
		t.Error("transferable should map to the flag bit")
	}
	if doc.Policy.ValidFrom != 1775001700 || doc.Policy.ValidTo != 1775002000 {
		t.Errorf("policy not carried over: %+v", doc.Policy)
	}
	if err := doc.Validate(); err != nil {
		t.Errorf("a document built from a valid request should validate: %v", err)
	}

	// transferable=false must leave the bitfield clear, not set some other bit.
	info.Transferable = false
	if flags := info.ToDocument().Flags; flags != 0 {
		t.Errorf("expected no flags set, got %d", flags)
	}
}

// setProperties and properties are absent from the JSON unless set, so existing
// clients send byte-identical request bodies.
func TestTransactionRequestOmitsPropertiesWhenUnset(t *testing.T) {
	b, err := json.Marshal(models.TransactionTokenDetails{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := decoded["properties"]; present {
		t.Error("properties must be omitted from the request when unset")
	}
	if v, ok := decoded["setProperties"].(bool); !ok || v {
		t.Errorf("setProperties should default to false, got %v", decoded["setProperties"])
	}
}

func TestIsPropertiesSet(t *testing.T) {
	var req models.TransactionRequest
	if req.IsPropertiesSet() {
		t.Error("a bare request must not be a properties write")
	}
	req.Tokens.SetProperties = true
	if !req.IsPropertiesSet() {
		t.Error("setProperties=true must be reported as a properties write")
	}
}

// The deployer is bootstrapped as an admin, so a document is never left with
// nobody able to edit it.
func TestPropertiesRequestDecodesFromJSON(t *testing.T) {
	const body = `{
		"setProperties": true,
		"nft": [{"nftId": "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco"}],
		"properties": {
			"transferable": true,
			"whitelist": ["did-a", "did-b"],
			"validTo": 5000
		}
	}`
	var details models.TransactionTokenDetails
	if err := json.Unmarshal([]byte(body), &details); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !details.SetProperties {
		t.Error("setProperties should decode as true")
	}
	if details.Properties == nil {
		t.Fatal("properties object should decode")
	}
	if len(details.Properties.Whitelist) != 2 {
		t.Errorf("expected 2 whitelist entries, got %v", details.Properties.Whitelist)
	}
	if !details.Properties.Transferable || details.Properties.ValidTo != 5000 {
		t.Errorf("properties fields not decoded: %+v", details.Properties)
	}
}
