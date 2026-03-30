package api

import (
	"github.com/rubixchain/rubixgoplatform/core/ipfsport"
	rubixsync "github.com/rubixchain/rubixgoplatform/core/sync"
	"github.com/rubixchain/rubixgoplatform/core/wallet"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func SetupAPI(listener *ipfsport.Listener, w *wallet.Wallet, log logger.Logger) {
	listener.AddRoute(rubixsync.APISyncTransactionChain, "POST", func(req *ensweb.Request) *ensweb.Result {
		return rubixsync.SyncTransactionChain(req, listener, w, log)
	})
}
