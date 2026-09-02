package util

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	mbase "github.com/multiformats/go-multibase"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// publishedMessage is one captured `pubsub/pub` call against the fake IPFS API.
type publishedMessage struct {
	topic   string
	payload []byte
}

// fakeIPFS stands in for the IPFS HTTP API so PublishTransaction can be
// exercised without a daemon. It records every pubsub/pub call, decoding the
// multibase-wrapped topic from the `arg` query parameter and the JSON payload
// from the multipart body — the exact wire shape go-ipfs-api produces.
type fakeIPFS struct {
	mu   sync.Mutex
	msgs []publishedMessage
}

func (f *fakeIPFS) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/pubsub/pub" {
			http.NotFound(w, r)
			return
		}
		_, topic, err := mbase.Decode(r.URL.Query().Get("arg"))
		if err != nil {
			t.Errorf("fakeIPFS: failed to decode topic: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("fakeIPFS: failed to read multipart body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		part, err := mr.NextPart()
		if err != nil {
			t.Errorf("fakeIPFS: missing multipart part: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payload, err := io.ReadAll(part)
		if err != nil {
			t.Errorf("fakeIPFS: failed to read part: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f.mu.Lock()
		f.msgs = append(f.msgs, publishedMessage{topic: string(topic), payload: payload})
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (f *fakeIPFS) captured() []publishedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]publishedMessage, len(f.msgs))
	copy(out, f.msgs)
	return out
}

func newTestPubSub(t *testing.T) (*types.PubSub, *fakeIPFS) {
	t.Helper()
	fake := &fakeIPFS{}
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	ps, err := types.NewPubSub(
		ipfsnode.NewShell(srv.URL),
		logger.New(&logger.LoggerOptions{Output: []io.Writer{io.Discard}}),
	)
	if err != nil {
		t.Fatalf("NewPubSub: %v", err)
	}
	return ps, fake
}

func testTxnInfo(network string) *models.TransactionInfo {
	return &models.TransactionInfo{
		Network:   network,
		Initiator: "bafybmiinitiator",
		Owner:     "bafybmiowner",
		Epoch:     1735689600,
		Tokens: &models.TransactionTokens{
			RBT: []*models.TokenInfo{{
				TokenID:               "bafybmitoken",
				PreviousTransactionID: "",
				TokenValue:            1,
			}},
		},
	}
}

// TestPublishTransactionAllNetworks asserts every network mode — localnet
// included — publishes exactly one message on the shared rubix_txn topic.
func TestPublishTransactionAllNetworks(t *testing.T) {
	for _, network := range []string{
		constants.NetworkMode_Localnet,
		constants.NetworkMode_Testnet,
		constants.NetworkMode_Mainnet,
	} {
		t.Run(network, func(t *testing.T) {
			ps, fake := newTestPubSub(t)
			txInfo := testTxnInfo(network)

			txn, err := PublishTransaction(ps, txInfo, &models.Signature{InitiatorSignature: "c2ln"}, true, "")
			if err != nil {
				t.Fatalf("PublishTransaction returned err: %v", err)
			}
			if txn == nil {
				t.Fatal("PublishTransaction returned a nil transaction; the network is not publishing")
			}

			msgs := fake.captured()
			if len(msgs) != 1 {
				t.Fatalf("expected exactly 1 publish, got %d", len(msgs))
			}
			if msgs[0].topic != constants.Event_RubixTxns {
				t.Fatalf("published on topic %q, want %q", msgs[0].topic, constants.Event_RubixTxns)
			}
		})
	}
}

// TestPublishTransactionLocalnetPayloadShape walks the payload a localnet
// publish puts on the wire through the same decoding steps Core.TxnCallBack
// performs, so a subscriber on any network can consume it unchanged.
func TestPublishTransactionLocalnetPayloadShape(t *testing.T) {
	ps, fake := newTestPubSub(t)
	txInfo := testTxnInfo(constants.NetworkMode_Localnet)
	signature := &models.Signature{InitiatorSignature: "c2ln"}

	txn, err := PublishTransaction(ps, txInfo, signature, true, "")
	if err != nil {
		t.Fatalf("PublishTransaction returned err: %v", err)
	}

	msgs := fake.captured()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 publish, got %d", len(msgs))
	}

	// Step 1 of TxnCallBack: the envelope unmarshals into an EventTransaction.
	var event models.EventTransaction
	if err := json.Unmarshal(msgs[0].payload, &event); err != nil {
		t.Fatalf("failed to unmarshal EventTransaction: %v", err)
	}
	if !event.Status {
		t.Error("Status is false; TxnCallBack drops the message before handling it")
	}
	if event.Transaction == nil {
		t.Fatal("Transaction is nil")
	}

	expectedID, err := GetTransactionID(txInfo)
	if err != nil {
		t.Fatalf("GetTransactionID: %v", err)
	}
	if event.TransactionID != expectedID || event.Transaction.ID != expectedID {
		t.Errorf("transaction ID mismatch: envelope %q / transaction %q, want %q",
			event.TransactionID, event.Transaction.ID, expectedID)
	}
	if txn.ID != expectedID {
		t.Errorf("returned transaction ID = %q, want %q", txn.ID, expectedID)
	}

	// Step 2 of TxnCallBack: Transaction.Info unmarshals into a TransactionInfo
	// carrying the fields the handler reads (Initiator drives AddPeerDetails).
	var decoded models.TransactionInfo
	if err := json.Unmarshal(event.Transaction.Info, &decoded); err != nil {
		t.Fatalf("failed to unmarshal TransactionInfo: %v", err)
	}
	if decoded.Network != constants.NetworkMode_Localnet {
		t.Errorf("Network = %q, want %q", decoded.Network, constants.NetworkMode_Localnet)
	}
	if decoded.Initiator != txInfo.Initiator {
		t.Errorf("Initiator = %q, want %q", decoded.Initiator, txInfo.Initiator)
	}
	if decoded.Owner != txInfo.Owner {
		t.Errorf("Owner = %q, want %q", decoded.Owner, txInfo.Owner)
	}
	if decoded.Tokens == nil || len(decoded.Tokens.RBT) != 1 ||
		decoded.Tokens.RBT[0].TokenID != txInfo.Tokens.RBT[0].TokenID {
		t.Errorf("RBT tokens did not round-trip: %+v", decoded.Tokens)
	}

	var decodedSig models.Signature
	if err := json.Unmarshal(event.Transaction.Signature, &decodedSig); err != nil {
		t.Fatalf("failed to unmarshal Signature: %v", err)
	}
	if decodedSig.InitiatorSignature != signature.InitiatorSignature {
		t.Errorf("InitiatorSignature = %q, want %q", decodedSig.InitiatorSignature, signature.InitiatorSignature)
	}
}

// TestPublishTransactionFailedLocalnetTxn covers the failure envelope: a
// localnet transaction that fails consensus must still publish, with Status
// false and the error message carried through.
func TestPublishTransactionFailedLocalnetTxn(t *testing.T) {
	ps, fake := newTestPubSub(t)

	const failureMsg = "consensus rejected"
	if _, err := PublishTransaction(ps, testTxnInfo(constants.NetworkMode_Localnet),
		&models.Signature{InitiatorSignature: "c2ln"}, false, failureMsg); err != nil {
		t.Fatalf("PublishTransaction returned err: %v", err)
	}

	msgs := fake.captured()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 publish, got %d", len(msgs))
	}
	var event models.EventTransaction
	if err := json.Unmarshal(msgs[0].payload, &event); err != nil {
		t.Fatalf("failed to unmarshal EventTransaction: %v", err)
	}
	if event.Status {
		t.Error("Status = true, want false for a failed transaction")
	}
	if event.Message != failureMsg {
		t.Errorf("Message = %q, want %q", event.Message, failureMsg)
	}
}
