package core

import "github.com/rubixchain/rubixgoplatform/core/model"

// Token is a lightweight token descriptor used in explorer/audit paths.
type Token struct {
	TokenHash  string  `json:"token_hash"`
	TokenValue float64 `json:"token_value"`
}

// AllToken holds per-token details for explorer submission.
type AllToken struct {
	TokenHash   string `json:"tokenHash"`
	BlockHash   string `json:"blockHash"`
	BlockNumber int    `json:"blockNumber"`
}

// QuorumDIDPeerMap maps a quorum DID to its peer connection details.
type QuorumDIDPeerMap struct {
	DID         string `json:"did"`
	DIDType     *int   `json:"did_type"`
	PeerID      string `json:"peer_id"`
	DIDLastChar string `json:"did_last_char"`
}

// ConensusRequest is the consensus request payload exchanged between initiator and quorums.
// Note: "Conensus" typo preserved for wire-format compatibility.
type ConensusRequest struct {
	ReqID              string       `json:"req_id"`
	Type               int          `json:"type"`
	Mode               int          `json:"mode"`
	SenderPeerID       string       `json:"sender_peerd_id"`
	ReceiverPeerID     string       `json:"receiver_peerd_id"`
	ContractBlock      []byte       `json:"contract_block"`
	QuorumList         []string     `json:"quorum_list"`
	DeployerPeerID     string       `json:"deployer_peerd_id"`
	SmartContractToken string       `json:"smart_contract_token"`
	ExecuterPeerID     string       `json:"executor_peer_id"`
	TransactionID      string       `json:"transaction_id"`
	TransactionEpoch   int          `json:"transaction_epoch"`
	PinningNodePeerID  string       `json:"pinning_node_peer_id"`
	NFT                string       `json:"nft"`
	FTinfo             model.FTInfo `json:"ft_info"`
	ExplorerDone       chan struct{} `json:"-"`
	OperationType      int          `json:"operation_type"`
}

// ConensusReply is the response from a quorum node after consensus evaluation.
// Note: "Conensus" typo preserved for wire-format compatibility.
type ConensusReply struct {
	ReqID     string   `json:"req_id"`
	Status    bool     `json:"status"`
	Message   string   `json:"message"`
	Result    []string `json:"result"`
	Hash      string   `json:"hash"`
	Signature []byte   `json:"signature"`
}
