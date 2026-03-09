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
	TokenID       string    `db:"token_id"`
	TransactionID string    `db:"transaction_id"`
	Role          int16     `db:"role"`
	Height        int64     `db:"height"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type DIDAlgo struct {
	ID       int16  `db:"id"`
	Name     string `db:"name"`
	IsActive bool   `db:"is_active"`
}

type DID struct {
	DID    string      `db:"did"`
	PeerID pgtype.Text `db:"peer_id"`
	Local  bool        `db:"local"`
	AlgoID pgtype.Int2 `db:"algo_id"`
}

type TokenStatus struct {
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
	TokenID        string         `db:"token_id"`
	ParentTokenID  pgtype.Text    `db:"parent_token_id"`
	TokenValue     pgtype.Numeric `db:"token_value"`
	TokenStatus    int16          `db:"token_status"`
	DID            string         `db:"did"`
	TransactionID  string         `db:"transaction_id"`
	TokenStateHash string         `db:"token_state_hash"`
	TokenType      int16          `db:"token_type"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
}

type TokenProviderMap struct {
	Token         string         `db:"token"`
	DID           pgtype.Text    `db:"did"`
	FuncID        pgtype.Int4    `db:"func_id"`
	Role          pgtype.Int4    `db:"role"`
	TransactionID pgtype.Text    `db:"transaction_id"`
	Sender        pgtype.Text    `db:"sender"`
	Receiver      pgtype.Text    `db:"receiver"`
	TokenValue    pgtype.Numeric `db:"token_value"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type UnpledgeSequenceInfo struct {
	TxID         string      `db:"tx_id"`
	PledgeTokens pgtype.Text `db:"pledge_tokens"`
	Epoch        pgtype.Int4 `db:"epoch"`
	QuorumDID    pgtype.Text `db:"quorum_did"`
	CreatedAt    time.Time   `db:"created_at"`
	UpdatedAt    time.Time   `db:"updated_at"`
}

type FT struct {
	ID         string      `db:"id"`
	FTName     pgtype.Text `db:"ft_name"`
	FTCount    pgtype.Int4 `db:"ft_count"`
	CreatorDID pgtype.Text `db:"creator_did"`
	CreatedAt  time.Time   `db:"created_at"`
	UpdatedAt  time.Time   `db:"updated_at"`
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
	Address   string    `db:"address"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Request struct {
	ID            string    `db:"id"`
	TransactionID string    `db:"transaction_id"`
	Status        int16     `db:"status"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}
