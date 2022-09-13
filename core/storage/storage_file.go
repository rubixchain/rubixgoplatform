package storage

import (
	"github.com/EnsurityTechnologies/adapter"
	"github.com/EnsurityTechnologies/config"
	"github.com/EnsurityTechnologies/uuid"
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
func (s *StorageFile) Init(storageName string, vaule interface{}) error {
	return s.ad.InitTable(storageName, vaule)
}

// Write will write into storage
func (s *StorageFile) Write(storageName string, vaule interface{}) error {
	return s.ad.Create(storageName, vaule)
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

// Close will close the stroage BD
func (s *StorageFile) Close() error {
	return s.ad.GetDB().Close()
}
