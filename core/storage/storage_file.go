package storage

import (
	"github.com/rubixchain/rubixgoplatform/wrapper/adapter"
	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type StorageFile struct {
	ad *adapter.Adapter
}

func NewStorageFile(cfg *config.Config) (*StorageFile, error) {
	ad, err := adapter.NewAdapter(cfg)
	if err != nil {
		return nil, err
	}
	s := &StorageFile{
		ad: ad,
	}
	return s, nil
}

// Init will initialize storage
func (s *StorageFile) Init(storageName string, vaule interface{}, force bool) error {
	return s.ad.InitTable(storageName, vaule, force)
}

// Write will write into storage
func (s *StorageFile) Write(storageName string, vaule interface{}) error {
	return s.ad.Create(storageName, vaule)
}

// Write will write into storage
func (s *StorageFile) WriteBatch(storageName string, value interface{}, batchSize int) error {
	return s.ad.CreateInBatches(storageName, value, batchSize)
}

// Update will update the storage
func (s *StorageFile) Update(stroageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.UpdateNew(uuid.Nil, stroageName, querryString, vaule, querryVaule)
}

// Delete will delet the data from the storage
func (s *StorageFile) Delete(stroageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.DeleteNew(uuid.Nil, stroageName, querryString, vaule, querryVaule)
}

// Read will read from the storage
func (s *StorageFile) Read(stroageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.FindNew(uuid.Nil, stroageName, querryString, vaule, querryVaule)
}

// Read will read from the storage
func (s *StorageFile) ReadWithOffset(stroageName string, offset int, limit int, value interface{}, querryString string, querryVaule ...interface{}) error {
	return s.ad.FindWithOffset(uuid.Nil, stroageName, querryString, value, offset, limit, querryVaule...)
}

func (s *StorageFile) GetDataCount(stroageName string, querryString string, querryVaule ...interface{}) int64 {
	return s.ad.GetCount(uuid.Nil, stroageName, querryString, querryVaule...)
}

// Close will close the stroage BD
func (s *StorageFile) Close() error {
	db, err := s.ad.GetDB().DB()
	if err != nil {
		return err
	}
	return db.Close()
}
