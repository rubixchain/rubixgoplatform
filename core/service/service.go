package service

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/EnsurityTechnologies/logger"
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

const (
	ArbitrationDIDTable  string = "DIDArbitration"
	ArbitrationTable     string = "Arbitration"
	ArbitrationTempTable string = "ArbitrationTemp"
	AribitrationLocked   string = "AribirationLocked"
	HashTable            string = "HashTable"
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

type HashEntry struct {
	Hash  string `gorm:"column:hash;primary_key"`
	Value int    `gorm:"column:value"`
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
	err = s.Init(HashTable, &HashEntry{})
	if err != nil {
		srv.log.Error("Failed to create hash table")
	}
	// var he HashEntry
	// err = s.Read(HashTable, &he, "value=?", 0)
	// if err != nil {
	// 	go srv.CalculateHash()
	// }
	return srv, nil
}

func (s *Service) CalculateHash() {
	for i := 0; i < 5000000; i++ {
		hash := sha256.Sum256([]byte(strconv.Itoa(i)))
		hashString := fmt.Sprintf("%x", hash)
		he := HashEntry{
			Hash:  hashString,
			Value: i,
		}
		err := s.s.Write(HashTable, &he)
		if err != nil {
			s.log.Error("Failed to write hash tbale", "err", err)
			return
		}
		if i%1000 == 0 {
			s.log.Info("Hash Calulation in progress...", "count", i)
		}
	}
}

func (s *Service) GetTokenNumber(hash string) (int, error) {
	var he HashEntry
	err := s.s.Read(HashTable, &he, "hash=?", hash)
	if err != nil {
		s.log.Error("Failed to get the token number", "hash", hash)
		return 0, err
	}
	return he.Value, nil
}

func (s *Service) GetTokenDetials(t string) (*TokenDetials, error) {
	var td TokenDetials
	err := s.s.Read(ArbitrationTable, &td, "token=?", t)
	if err != nil {
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
			err = s.s.Delete(ArbitrationTempTable, &td, "token=?", td.Token)
			if err != nil {
				s.log.Error("Failed to delete from arbitary temp table", "err", err)
				return err
			}
		} else {
			break
		}
	}
	return nil
}

func (s *Service) UpdateTempTokenDetials(td *TokenDetials) error {
	err := s.s.Write(ArbitrationTempTable, td)
	if err != nil {
		var t TokenDetials
		err = s.s.Read(ArbitrationTempTable, &t, "token=?", td.Token)
		if err != nil {
			s.log.Error("Failed to write aribitration temp table", "err", err)
			return err
		}
		err = s.s.Delete(ArbitrationTempTable, &TokenDetials{}, "did=?", t.DID)
		if err != nil {
			s.log.Error("Failed to write aribitration temp table", "err", err)
			return err
		}
		err := s.s.Write(ArbitrationTempTable, td)
		if err != nil {
			s.log.Error("Failed to write aribitration temp table", "err", err)
			return err
		}
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
