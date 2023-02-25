package service

import (
	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	ArbitrationDIDTable  string = "DIDArbitration"
	ArbitrationTable     string = "Arbitration"
	ArbitrationTempTable string = "ArbitrationTemp"
	AribitrationLocked   string = "AribirationLocked"
)

type Service struct {
	s   storage.Storage
	log logger.Logger
}

type DIDMap struct {
	OldDID string `gorm:"column:old_did;primary_key"`
	NewDID string `gorm:"column:new_did"`
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
	err = s.Init(ArbitrationTempTable, &TokenDetials{})
	if err != nil {
		srv.log.Error("Failed to init temp arbitration")
	}
	err = s.Init(ArbitrationDIDTable, &DIDMap{})
	if err != nil {
		srv.log.Error("Failed to init did arbitration")
	}
	err = s.Init(AribitrationLocked, &TokenDetials{})
	if err != nil {
		srv.log.Error("Failed to init arbitration locked table")
	}
	return srv, nil
}

func (s *Service) GetTokenDetials(t string) (*TokenDetials, error) {
	var td TokenDetials
	err := s.s.Read(ArbitrationTable, &td, "token=?", t)
	if err != nil {
		s.log.Error("Failed to read aribitration table", "err", err)
		return nil, err
	}
	return &td, nil
}

func (s *Service) UpdateTokenDetials(did string) error {
	var td TokenDetials
	for {
		err := s.s.Read(ArbitrationTempTable, &td, "did=?", did)
		if err != nil {
			break
		}
		if td.Token != "" {
			err = s.s.Write(ArbitrationTable, &td)
			if err != nil {
				s.log.Error("Failed to write arbitary table", "err", err)
				return err
			}
		}
	}

	err := s.s.Write(ArbitrationTable, td)
	if err != nil {
		s.log.Error("Failed to write aribitration table", "err", err)
	}
	return err
}

func (s *Service) UpdateTempTokenDetials(td *TokenDetials) error {
	err := s.s.Write(ArbitrationTempTable, td)
	if err != nil {
		s.log.Error("Failed to write aribitration table", "err", err)
	}
	return err
}

func (s *Service) UpdateDIDMap(dm *DIDMap) error {
	err := s.s.Write(ArbitrationDIDTable, dm)
	if err != nil {
		s.log.Error("Failed to write did aribitration table", "err", err)
	}
	return err
}

func (s *Service) GetDIDMap(did string) (*DIDMap, error) {
	var dm DIDMap
	err := s.s.Read(ArbitrationDIDTable, &dm, "old_did=?", did)
	if err != nil {
		return nil, err
	}
	return &dm, nil
}

func (s *Service) IsDIDExist(did string) bool {
	dm, err := s.GetDIDMap(did)
	if err != nil {
		return false
	}
	return dm.OldDID == did
}

func (s *Service) AddLockedTokens(ts []string) error {
	for _, t := range ts {
		dt := TokenDetials{
			Token: t,
		}
		err := s.s.Write(AribitrationLocked, &dt)
		if err != nil {
			s.log.Error("Token failed to write lock table", "err", err, "token", t)
			return err
		}
	}
	return nil
}
