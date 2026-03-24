package core

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types"
)

// publishTxn publishes a transaction event to the network.
// TODO(phase07): implement pubsub broadcast via c.ps.
func (c *Core) publishTxn(txnInfo *model.PubSubTxnInfo) error {
	return nil
}

// initiateConsensus drives the quorum consensus round for a transaction.
// TODO(phase07): wire up to the real consensus package.
func (c *Core) initiateConsensus(req *ConensusRequest, contract *ConsensusContract, dc types.DIDCrypto) (*model.TransactionDetails, interface{}, interface{}, error) {
	return &model.TransactionDetails{}, nil, nil, nil
}
