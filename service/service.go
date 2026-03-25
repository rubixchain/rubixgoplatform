// Package service provides stub implementations of the token arbitration service.
// TODO(phase07): implement real service layer backed by PostgreSQL.
package service

// DIDMap maps an old DID to a new DID for arbitration/migration.
type DIDMap struct {
	OldDID string `json:"old_did"`
	NewDID string `json:"new_did"`
}

// TokenDetails holds token details returned by the service layer.
type TokenDetails struct {
	Token string `json:"token"`
	DID   string `json:"did"`
}

// Service provides arbitration and migration operations.
// TODO(phase07): back by PostgreSQL queries.
type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) UpdateTokenDetials(did string) error { return nil }

func (s *Service) UpdateDIDMap(dm *DIDMap) error { return nil }

func (s *Service) IsDIDExist(did string) bool { return false }

func (s *Service) GetTokenNumber(tokenHash string) (int, error) { return 0, nil }

func (s *Service) GetTokenDetials(token string) (*TokenDetails, error) { return nil, nil }
