package models_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// Hash-compatibility guard for TransactionInfo, the canonical signed payload:
// its transaction ID is hex(SHA3-256(json.Marshal(txInfo))), so any change to
// the emitted JSON changes the ID of transactions unrelated to that change.
//
// The literals below are hardcoded legacy bytes and hashes, deliberately not
// recomputed from the live structs. If a change fails this test, the change is
// network-breaking — do not update the literals to make it pass.

type hashCase struct {
	name string
	info *models.TransactionInfo
	// json is the exact byte output legacy nodes produce for this payload.
	json string
	// txID is hex(SHA3-256(json)) — what legacy nodes sign.
	txID string
}

func hashCases() []hashCase {
	return []hashCase{
		{
			// The case a non-omitempty array breaks most visibly: it would
			// append ,"properties":null to every transaction.
			name: "zero value",
			info: &models.TransactionInfo{},
			json: `{"initiator":"","owner":"","epoch":0,"network":"","tokens":null,"committedTokens":null,"quorums":null,"memo":""}`,
			txID: "01917d24f4033b51535b9be57ff7b4e292debd325d9ba7c9c35caa7371b8df9a",
		},
		{
			// Populated tokens object with the four legacy arrays present.
			name: "rbt transfer",
			info: &models.TransactionInfo{
				Initiator: "bafybmiinitiatordid",
				Owner:     "bafybmiownerdid",
				Epoch:     1755500000,
				Network:   "testnet",
				Tokens: &models.TransactionTokens{
					RBT: []*models.TokenInfo{
						{
							TokenID:               "QmRBTTokenOne",
							PreviousTransactionID: "prevtxn1",
							Data:                  "",
							TokenValue:            0.001,
						},
					},
					NFT:           []*models.TokenInfo{},
					FT:            []*models.TokenInfo{},
					SmartContract: []*models.TokenInfo{},
				},
				CommittedTokens: []*models.TokenInfo{
					{
						TokenID:               "QmCommittedOne",
						PreviousTransactionID: "",
						Data:                  "",
						TokenValue:            0.001,
					},
				},
				Quorums: []*models.QuorumInfo{
					{
						Did: "bafybmiquorumdid",
						Tokens: []*models.TokenInfo{
							{
								TokenID:               "QmPledged",
								PreviousTransactionID: "prevpledge",
								Data:                  "",
								TokenValue:            1,
							},
						},
					},
				},
				Memo: "transfer memo",
			},
			json: `{"initiator":"bafybmiinitiatordid","owner":"bafybmiownerdid","epoch":1755500000,"network":"testnet","tokens":{"rbt":[{"tokenId":"QmRBTTokenOne","previousTransactionID":"prevtxn1","data":"","tokenValue":0.001}],"nft":[],"ft":[],"smartContract":[]},"committedTokens":[{"tokenId":"QmCommittedOne","previousTransactionID":"","data":"","tokenValue":0.001}],"quorums":[{"did":"bafybmiquorumdid","tokens":[{"tokenId":"QmPledged","previousTransactionID":"prevpledge","data":"","tokenValue":1}]}],"memo":"transfer memo"}`,
			txID: "15a805c783d55227c214cf8ea92bca1bdc4db955e47b3fb80f9e6cf18d0c3589",
		},
		{
			// Nil and empty slices serialize differently (null vs []), so
			// both shapes are pinned.
			name: "nft execute with nil siblings",
			info: &models.TransactionInfo{
				Initiator: "bafybmiinitiatordid",
				Owner:     "bafybmiownerdid",
				Epoch:     1755500001,
				Network:   "testnet",
				Tokens: &models.TransactionTokens{
					NFT: []*models.TokenInfo{
						{
							TokenID:               "QmNFTTokenOne",
							PreviousTransactionID: "prevnft1",
							Data:                  "{\"k\":\"v\"}",
							TokenValue:            2.5,
						},
					},
				},
				Memo: "",
			},
			json: `{"initiator":"bafybmiinitiatordid","owner":"bafybmiownerdid","epoch":1755500001,"network":"testnet","tokens":{"rbt":null,"nft":[{"tokenId":"QmNFTTokenOne","previousTransactionID":"prevnft1","data":"{\"k\":\"v\"}","tokenValue":2.5}],"ft":null,"smartContract":null},"committedTokens":null,"quorums":null,"memo":""}`,
			txID: "dcfff5cc29933b5c903b21836b4bb723ad5b807b07cb172b8632ace541bdf978",
		},
		{
			// The struct the Properties array is added to, pinned alone.
			name: "empty tokens struct",
			info: &models.TransactionInfo{
				Tokens: &models.TransactionTokens{},
			},
			json: `{"initiator":"","owner":"","epoch":0,"network":"","tokens":{"rbt":null,"nft":null,"ft":null,"smartContract":null},"committedTokens":null,"quorums":null,"memo":""}`,
			txID: "60f725fb791e94cf38ebd6e08d7f1d167011e3b2c37758ba0e5228a235933b10",
		},
	}
}

// Asserts payloads carrying no properties serialize byte-identically to legacy.
func TestLegacyTransactionInfoJSONUnchanged(t *testing.T) {
	for _, tc := range hashCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := models.SerializeTransactionInfo(tc.info)
			if err != nil {
				t.Fatalf("SerializeTransactionInfo: %v", err)
			}
			if string(got) != tc.json {
				t.Errorf("legacy JSON changed — this is a network-breaking change.\n want: %s\n  got: %s", tc.json, string(got))
			}
		})
	}
}

// Asserts the signed and persisted transaction ID is unchanged for legacy payloads.
func TestLegacyTransactionIDUnchanged(t *testing.T) {
	for _, tc := range hashCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := util.GetTransactionID(tc.info)
			if err != nil {
				t.Fatalf("GetTransactionID: %v", err)
			}
			if got != tc.txID {
				t.Errorf("legacy transaction ID changed — this is a network-breaking change.\n want: %s\n  got: %s", tc.txID, got)
			}
		})
	}
}

// Pins the field order of the tokens object: json.Marshal emits in declaration
// order, so reordering silently changes every transaction hash.
func TestTransactionTokensFieldOrder(t *testing.T) {
	b, err := json.Marshal(&models.TransactionTokens{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Properties is absent despite being declared: it is empty, and omitempty
	// drops it. That absence is what keeps legacy hashes intact.
	const want = `{"rbt":null,"nft":null,"ft":null,"smartContract":null}`
	if string(b) != want {
		t.Errorf("TransactionTokens wire shape changed.\n want: %s\n  got: %s", want, string(b))
	}
}

// Asserts the omitempty tag is present. The hash tests would also catch its
// removal, but this names the cause directly.
func TestPropertiesOmitEmptyTag(t *testing.T) {
	f, ok := reflect.TypeOf(models.TransactionTokens{}).FieldByName("Properties")
	if !ok {
		t.Fatal("TransactionTokens has no Properties field")
	}
	if got := f.Tag.Get("json"); got != "properties,omitempty" {
		t.Errorf("Properties json tag must be `properties,omitempty`, got %q — "+
			"without omitempty every transaction on the network changes hash", got)
	}
	// Must also stay last: inserting it earlier would reorder legacy fields on
	// any transaction that does carry properties.
	tt := reflect.TypeOf(models.TransactionTokens{})
	if last := tt.Field(tt.NumField() - 1).Name; last != "Properties" {
		t.Errorf("Properties must be the last field of TransactionTokens, but %q is", last)
	}
}

// Asserts omitempty does not make populated properties tokens invisible.
func TestPropertiesSerializesWhenPresent(t *testing.T) {
	info := &models.TransactionInfo{
		Tokens: &models.TransactionTokens{
			Properties: []*models.TokenInfo{
				{TokenID: "QmPropsToken", PreviousTransactionID: "prev", TokenValue: 0.001},
			},
		},
	}
	b, err := models.SerializeTransactionInfo(info)
	if err != nil {
		t.Fatalf("SerializeTransactionInfo: %v", err)
	}
	const want = `{"initiator":"","owner":"","epoch":0,"network":"","tokens":{"rbt":null,"nft":null,"ft":null,"smartContract":null,"properties":[{"tokenId":"QmPropsToken","previousTransactionID":"prev","data":"","tokenValue":0.001}]},"committedTokens":null,"quorums":null,"memo":""}`
	if string(b) != want {
		t.Errorf("properties payload encoding mismatch.\n want: %s\n  got: %s", want, string(b))
	}
}
