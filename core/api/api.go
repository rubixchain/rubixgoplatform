package api

import (
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

const APISyncTransactionChain = "/api/sync-transaction-chain"

func SetupAPI(listener *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) {
	listener.AddRoute(APISyncTransactionChain, "POST", func(req *ensweb.Request) *ensweb.Result {
		return SyncTransactionChain(req, listener, w, log)
	})
}
