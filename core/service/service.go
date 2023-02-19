package service

import (
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	ArbitrationTable string = "Arbitration"
)

type Service struct {
	s   storage.Storage
	log logger.Logger
}

type TokenDetials struct {
	Token string `gorm:"column:token;primary_key"`
	DID   string `gorm:"column:did"`
}

func NewService(s storage.Storage, log logger.Logger) (*Service, error) {
	srv := &Service{
		s:   s,
		log: log.Named("service"),
	}
	// Initialize the Arbitration Table to store the token
	// detials
	err := s.Init(ArbitrationTable, &TokenDetials{})
	if err != nil {
		srv.log.Error("Failed to init arbitration")
	}
	return srv, nil
}

func (s *Service) GetTokenDetials(t string) (*TokenDetials, error) {
	var td TokenDetials
	err := s.s.Read(ArbitrationTable, &td, "token=?", t)
	if err != nil {
		return nil, err
	}
	return &td, nil
}

func (s *Service) UpdateTokenDetials(td *TokenDetials) error {
	return s.s.Write(ArbitrationTable, &td)
}
