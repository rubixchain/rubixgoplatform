package models

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Transactions struct {
	ID        string          `db:"id"`
	Info      json.RawMessage `db:"info"`
	Signature json.RawMessage `db:"signature"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

type TokenChain struct {
	ID                    int32     `db:"id"`
	TokenID               string    `db:"token_id"`
	TransactionID         string    `db:"transaction_id"`
	PreviousTransactionID *string   `db:"previous_transaction_id"`
	Role                  int16     `db:"role"`
	Position              int64     `db:"position"`
	CreatedAt             time.Time `db:"created_at"`
	UpdatedAt             time.Time `db:"updated_at"`
}

type DIDAlgo struct {
	ID       int16  `db:"id"`
	Name     string `db:"name"`
	IsActive bool   `db:"is_active"`
}

type DID struct {
	DID    string `db:"did"`
	PeerID string `db:"peer_id"`
	Local  bool   `db:"local"`
	AlgoID int64  `db:"algo_id"`
}

type TokenRole struct {
	ID       int16  `db:"id"`
	Name     string `db:"name"`
	IsActive bool   `db:"is_active"`
}

type TokenType struct {
	ID       int16  `db:"id"`
	Name     string `db:"name"`
	IsActive bool   `db:"is_active"`
}

type Token struct {
	TokenID        string      `db:"token_id"`
	ParentTokenID  pgtype.Text `db:"parent_token_id"`
	TokenValue     float64     `db:"token_value"`
	TokenStatus    int16       `db:"token_status"`
	DID            string      `db:"did"`
	TransactionID  string      `db:"transaction_id"`
	TokenStateHash string      `db:"token_state_hash"`
	TokenType      int16       `db:"token_type"`
	LatestPosition int64       `db:"latest_position"`
	LatestRole     int16       `db:"latest_role"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
	SyncStatus     int         `db:"-"` // transient field, not persisted
}

type TransactionUnit struct {
	TransactionID string    `db:"transaction_id"`
	DID           string    `db:"did"`
	ExecutionRole string    `db:"execution_role"`
	Status        string    `db:"status"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type TokenProviderMap struct {
	Token         string         `db:"token"`
	DID           pgtype.Text    `db:"did"`
	FuncID        pgtype.Int4    `db:"func_id"`
	Role          pgtype.Int4    `db:"role"`
	TransactionID pgtype.Text    `db:"transaction_id"`
	Initiator     pgtype.Text    `db:"initiator"`
	Owner         pgtype.Text    `db:"owner"`
	TokenValue    pgtype.Numeric `db:"token_value"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type UnpledgeSequenceInfo struct {
	TxID         string    `db:"tx_id"`
	PledgeTokens []string  `db:"pledge_tokens"`
	Epoch        int       `db:"epoch"`
	QuorumDID    string    `db:"quorum_did"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type FT struct {
	FTName     string    `db:"ft_name"`
	FTCount    int64     `db:"ft_count"`
	CreatorDID string    `db:"creator_did"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type TokenRecovery struct {
	TransactionID string             `db:"transaction_id"`
	RecoveredAt   pgtype.Timestamptz `db:"recovered_at"`
	RecoveredBy   pgtype.Text        `db:"recovered_by"`
	TokenCount    pgtype.Int4        `db:"token_count"`
	TokenIDs      pgtype.Text        `db:"token_ids"`
	RecoveryType  pgtype.Text        `db:"recovery_type"`
	RecoveryNotes pgtype.Text        `db:"recovery_notes"`
	CreatedAt     time.Time          `db:"created_at"`
	UpdatedAt     time.Time          `db:"updated_at"`
}

type LocalTestTokenInfo struct {
	Attribute string      `db:"attribute"`
	Value     pgtype.Int4 `db:"value"`
	CreatedAt time.Time   `db:"created_at"`
	UpdatedAt time.Time   `db:"updated_at"`
}

type CallBackURL struct {
	SmartContractHash string      `db:"smart_contract_hash"`
	CallbackURL       pgtype.Text `db:"callback_url"`
	CreatedAt         time.Time   `db:"created_at"`
	UpdatedAt         time.Time   `db:"updated_at"`
}

type TokenStateHash struct {
	DID            pgtype.Text `db:"did"`
	TokenStateHash string      `db:"token_state_hash"`
	PledgedToken   pgtype.Text `db:"pledged_token"`
	TransactionID  pgtype.Text `db:"transaction_id"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
}

type QuorumManager struct {
	Did       string    `db:"did"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Request struct {
	ID            string      `db:"id"`
	TransactionID pgtype.Text `db:"transaction_id"`
	Status        int16       `db:"status"`
	CreatedAt     time.Time   `db:"created_at"`
	UpdatedAt     time.Time   `db:"updated_at"`
}

type TokenDenom struct {
	ID         int64     `db:"id"`
	DID        string    `db:"did"`
	TokenDemom float64   `db:"denom"`
	Count      int64     `db:"count"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type TokenchainIndex struct {
	TokenID   string    `db:"token_id"`
	Index     []int     `db:"index"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type FullNodeTokenChain struct {
	TokenID       string    `db:"token_id"`
	TransactionID string    `db:"transaction_id"`
	Role          int16     `db:"role"`
	Height        int64     `db:"height"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type FullNodeTokenchainIndex struct {
	TokenID   string    `db:"token_id"`
	Index     []int     `db:"index"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type FullNodeRBT struct {
	TokenID        string      `db:"token_id"`
	ParentTokenID  pgtype.Text `db:"parent_token_id"`
	TokenValue     float64     `db:"token_value"`
	TokenStatus    int16       `db:"token_status"`
	DID            string      `db:"did"`
	TransactionID  string      `db:"transaction_id"`
	TokenStateHash string      `db:"token_state_hash"`
	TokenType      int16       `db:"token_type"`
	LatestPosition int64       `db:"latest_position"`
	LatestRole     int16       `db:"latest_role"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
}

type FullNodeFT struct {
	TokenID        string    `db:"token_id"`
	TokenValue     float64   `db:"token_value"`
	TokenStatus    int16     `db:"token_status"`
	DID            string    `db:"did"`
	TransactionID  string    `db:"transaction_id"`
	TokenStateHash string    `db:"token_state_hash"`
	TokenType      int16     `db:"token_type"`
	LatestPosition int64     `db:"latest_position"`
	LatestRole     int16     `db:"latest_role"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type FullNodeNFT struct {
	TokenID        string    `db:"token_id"`
	TokenValue     float64   `db:"token_value"`
	TokenStatus    int16     `db:"token_status"`
	DID            string    `db:"did"`
	TransactionID  string    `db:"transaction_id"`
	TokenStateHash string    `db:"token_state_hash"`
	TokenType      int16     `db:"token_type"`
	LatestPosition int64     `db:"latest_position"`
	LatestRole     int16     `db:"latest_role"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type FullNodeSmartContract struct {
	TokenID        string    `db:"token_id"`
	TokenValue     float64   `db:"token_value"`
	TokenStatus    int16     `db:"token_status"`
	TransactionID  string    `db:"transaction_id"`
	TokenStateHash string    `db:"token_state_hash"`
	TokenType      int16     `db:"token_type"`
	LatestPosition int64     `db:"latest_position"`
	LatestRole     int16     `db:"latest_role"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// SyncTransactionChainRequest is used by the transaction chain sync API.
// TODO: dev-team -- expand fields as needed for full sync implementation
type SyncTransactionChainRequest struct {
	Did string `json:"did"`
}
type GenesisAndLatestTransactionSyncRequest struct {
	Token string `json:"token"`
}
type TransactionChainSyncRequest struct {
	TokenID       string `json:"token_id"`
	TransactionID string `json:"transaction_id"`
}
type TransactionChainSyncReply struct {
	Status            bool     `json:"status"`
	Message           string   `json:"message"`
	NextTransactionID string   `json:"next_transaction_id"`
	Transactions      [][]byte `json:"transactions"`
}
type GenesisAndLatestTransactionSyncReply struct {
	Status             bool   `json:"status"`
	Message            string `json:"message"`
	GenesisTransaction []byte `json:"genesis_transaction"`
	LatestTransaction  []byte `json:"latest_transaction"`
}
