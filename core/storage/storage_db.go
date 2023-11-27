package storage

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/adapter"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type StorageDB struct {
	ad *adapter.Adapter
}

func NewStorageDB(cfg *config.Config) (*StorageDB, error) {
	ad, err := adapter.NewAdapter(cfg)
	if err != nil {
		return nil, err
	}
	s := &StorageDB{
		ad: ad,
	}
	return s, nil
}

// Init will initialize storage
func (s *StorageDB) Init(storageName string, value interface{}, force bool) error {
	return s.ad.InitTable(storageName, value, force)
}

// Write will write into storage
func (s *StorageDB) Write(storageName string, value interface{}) error {
	return s.ad.Create(storageName, value)
}

// Write will write into storage
func (s *StorageDB) WriteBatch(storageName string, value interface{}, batchSize int) error {
	return s.ad.CreateInBatches(storageName, value, batchSize)
}

// Update will update the storage
func (s *StorageDB) Update(stroageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.SaveNew(uuid.Nil, stroageName, querryString, value, querryVaule...)
}

// Delete will delet the data from the storage
func (s *StorageDB) Delete(stroageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.DeleteNew(uuid.Nil, stroageName, querryString, value, querryVaule...)
}

// Read will read from the storage
func (s *StorageDB) Read(stroageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.FindNew(uuid.Nil, stroageName, querryString, value, querryVaule...)
}

// Read will read from the storage
func (s *StorageDB) ReadWithOffset(stroageName string, offset int, limit int, value interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.FindWithOffset(uuid.Nil, stroageName, querryString, value, offset, limit, querryVaule...)
}

func (s *StorageDB) GetDataCount(stroageName string, querryString string, querryVaule ...interface{}) int64 {
	return s.ad.GetCount(uuid.Nil, stroageName, querryString, querryVaule...)
}

// Close will close the stroage BD
func (s *StorageDB) Close() error {
	db, err := s.ad.GetDB().DB()
	if err != nil {
		return err
	}
	return db.Close()
}
