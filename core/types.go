package core

import (
	"github.com/rubixchain/rubixgoplatform/core/model"
)

// ConsensusStatus tracks the state of an in-progress consensus round.
// TODO(phase07): populate fields as consensus logic is migrated.
type ConsensusStatus struct{}

// PledgeDetails tracks token pledge information for a consensus request.
// TODO(phase07): populate fields as pledge logic is migrated.
type PledgeDetails struct{}

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

// QuorumData holds the type and address of a quorum node.
type QuorumData struct {
	Type    int    `json:"type"`
	Address string `json:"address"`
}

// SignatureRequest is the payload sent to a quorum node requesting a block signature.
type SignatureRequest struct {
	TokenChainBlock []byte `json:"token_chain_block"`
}

// SignatureReply is the response from a quorum node with the block signature.
type SignatureReply struct {
	model.BasicResponse
	Signature []byte `json:"signature"`
}

// UpdatePledgeRequest is the payload sent to update pledge token status on a quorum.
type UpdatePledgeRequest struct {
	TokenChainBlock             []byte   `json:"token_chain_block"`
	Mode                        int      `json:"mode"`
	PledgedTokens               []string `json:"pledged_tokens"`
	TransactionID               string   `json:"transaction_id"`
	TransactionEpoch            int64    `json:"transaction_epoch"`
	TransferredTokenStateHashes []string `json:"transferred_token_state_hashes"`
}

// CreditScore holds the credit scoring payload sent by/to a quorum node.
type CreditScore struct {
	Score int `json:"score"`
}

// SendTokenRequest is the payload received by the receiver when tokens are being sent.
type SendTokenRequest struct {
	Address            string             `json:"address"`
	TokenInfo          []ContractTokenInfo `json:"token_info"`
	TokenChainBlock    []byte             `json:"token_chain_block"`
	QuorumList         []string           `json:"quorum_list"`
	QuorumInfo         []QuorumDIDPeerMap `json:"quorum_info"`
	TransactionEpoch   int                `json:"transaction_epoch"`
	PinningServiceMode bool               `json:"pinning_service_mode"`
	FTInfo             model.FTInfo       `json:"ft_info"`
}

// TokenList is a payload containing a DID and a list of token hashes.
type TokenList struct {
	DID    string   `json:"did"`
	Tokens []string `json:"tokens"`
}

// QuorumManager manages the set of quorum nodes.
// TODO(phase07): implement backed by PostgreSQL.
type QuorumManager struct{}

func (qm *QuorumManager) AddQuorum(qds []QuorumData) error { return nil }
