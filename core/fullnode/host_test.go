package fullnode

import (
	"fmt"
	"io"

	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// testHost stands in for the node. It supplies the discard logger the code under
// test writes to; every other method fails, because nothing these tests exercise
// is supposed to reach the node around the pipeline. This replaces the newTestCore
// helper, which existed for the same reason: to supply a logger and nothing else.
type testHost struct{}

var _ Host = (*testHost)(nil)

func newTestHost() *testHost { return &testHost{} }

func (h *testHost) Log() logger.Logger {
	return logger.New(&logger.LoggerOptions{Output: []io.Writer{io.Discard}})
}

func (h *testHost) Wallet() *wallet.Wallet { panic("testHost: no wallet in unit tests") }
func (h *testHost) Listener() *ipfsport.Listener {
	panic("testHost: no listener in unit tests")
}
func (h *testHost) IsFullNode() bool                 { return true }
func (h *testHost) NetworkFlags() (bool, bool, bool) { return true, false, false }

func (h *testHost) InitialiseDID(string) (types.DIDCrypto, error) {
	return nil, fmt.Errorf("testHost: InitialiseDID not available in unit tests")
}

func (h *testHost) SyncTransactionChainsFromPeer(string, []string, map[string]string, []string, bool, bool) error {
	return fmt.Errorf("testHost: SyncTransactionChainsFromPeer not available in unit tests")
}

func (h *testHost) SyncTokensFromFullnode([]string) (map[string]string, error) {
	return nil, fmt.Errorf("testHost: SyncTokensFromFullnode not available in unit tests")
}

func (h *testHost) FetchGenesisTransactionFromPeer(string, string) (*models.Transactions, error) {
	return nil, fmt.Errorf("testHost: FetchGenesisTransactionFromPeer not available in unit tests")
}

func (h *testHost) GetTransactionInfoByID(string) (*models.TransactionInfo, error) {
	return nil, fmt.Errorf("testHost: GetTransactionInfoByID not available in unit tests")
}

func (h *testHost) GetParentBurnTxID(string) (string, bool, error) {
	return "", false, fmt.Errorf("testHost: GetParentBurnTxID not available in unit tests")
}

func (h *testHost) CheckTokenStateHashPinned(string, string) error {
	return fmt.Errorf("testHost: CheckTokenStateHashPinned not available in unit tests")
}

func (h *testHost) CPUUsage(map[string]uint64) (float64, map[string]uint64) {
	return 0, nil
}

func (h *testHost) MemoryUsagePercent() float64 { return 0 }
