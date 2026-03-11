package wallet

import (
	"strconv"
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

	var tokenValue float64
	f8, err := m.TokenValue.Float64Value()
	if err == nil {
		tokenValue = f8.Float64
	}

	var latestRole int16
	if m.LatestRole.Valid {
		latestRole = m.LatestRole.Int16
	}

	return Token{
		TokenID:        m.TokenID,
		ParentTokenID:  parentTokenID,
		TokenValue:     tokenValue,
		TokenStatus:    m.TokenStatus,
		DID:            m.DID,
		TransactionID:  m.TransactionID,
		TokenStateHash: m.TokenStateHash,
		TokenType:      m.TokenType,
		LatestPosition: m.LatestPosition,
		LatestRole:     latestRole,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func (t Token) ToModel() models.Token {
	var n pgtype.Numeric
	n.Scan(strconv.FormatFloat(t.TokenValue, 'f', -1, 64)) //nolint:errcheck

	return models.Token{
		TokenID: t.TokenID,
		ParentTokenID: pgtype.Text{
			String: t.ParentTokenID,
			Valid:  t.ParentTokenID != "",
		},
		TokenValue:     n,
		TokenStatus:    t.TokenStatus,
		DID:            t.DID,
		TransactionID:  t.TransactionID,
		TokenStateHash: t.TokenStateHash,
		TokenType:      t.TokenType,
		LatestPosition: t.LatestPosition,
		LatestRole: pgtype.Int2{
			Int16: t.LatestRole,
			Valid: t.LatestRole != 0,
		},
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}
