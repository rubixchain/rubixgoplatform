package wallet

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rubixchain/rubixgoplatform/types/models"
)

type Token struct {
	TokenID        string
	ParentTokenID  string
	TokenValue     float64
	TokenStatus    int16
	DID            string
	TransactionID  string
	TokenStateHash string
	TokenType      int16
	LatestPosition int64
	LatestRole     int16
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func TokenFromModel(m models.Token) Token {
	var parentTokenID string
	if m.ParentTokenID.Valid {
		parentTokenID = m.ParentTokenID.String
	}

	return Token{
		TokenID:        m.TokenID,
		ParentTokenID:  parentTokenID,
		TokenValue:     m.TokenValue,
		TokenStatus:    m.TokenStatus,
		DID:            m.DID,
		TransactionID:  m.TransactionID,
		TokenStateHash: m.TokenStateHash,
		TokenType:      m.TokenType,
		LatestPosition: m.LatestPosition,
		LatestRole:     m.LatestRole,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func (t Token) ToModel() models.Token {
	return models.Token{
		TokenID: t.TokenID,
		ParentTokenID: pgtype.Text{
			String: t.ParentTokenID,
			Valid:  t.ParentTokenID != "",
		},
		TokenValue:     t.TokenValue,
		TokenStatus:    t.TokenStatus,
		DID:            t.DID,
		TransactionID:  t.TransactionID,
		TokenStateHash: t.TokenStateHash,
		TokenType:      t.TokenType,
		LatestPosition: t.LatestPosition,
		LatestRole:     t.LatestRole,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}
