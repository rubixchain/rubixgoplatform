package api

import (
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// SetupAPI registers peer-facing API routes.
// The sync-transaction-chain route has been moved to Core.TransactionSetup().
func SetupAPI(listener *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) {
}
