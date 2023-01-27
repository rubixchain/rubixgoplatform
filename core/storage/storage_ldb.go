package storage

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

type StorageLDB struct {
	dirPath string
	sm      map[string]bool
	l       sync.Mutex
}

func NewStorageLDB(dirPath string) (*StorageLDB, error) {

	s := &StorageLDB{
		dirPath: dirPath,
		sm:      make(map[string]bool),
	}
	return s, nil
}

// Init will initialize storage
func (s *StorageLDB) Init(storageName string, value interface{}) error {
	s.l.Lock()
	_, ok := s.sm[storageName]
	if !ok {
		s.sm[storageName] = false
	}
	s.l.Unlock()
	return nil
}

func (s *StorageLDB) getStorage(storageName string) {
	wait := true
	for {
		s.l.Lock()
		status, ok := s.sm[storageName]
		if ok {
			if !status {
				s.sm[storageName] = true
				wait = false
			}
		} else {
			s.sm[storageName] = true
			wait = false
		}
		s.l.Unlock()
		if !wait {
			break
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func (s *StorageLDB) releaseStorage(storageName string) {
	s.l.Lock()
	_, ok := s.sm[storageName]
	if ok {
		s.sm[storageName] = false
	}
	s.l.Unlock()
}

// Write will write into storage
func (s *StorageLDB) Write(storageName string, value interface{}) error {
	v, ok := value.(*StorageType)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	s.getStorage(storageName)
	defer s.releaseStorage(storageName)
	db, err := leveldb.OpenFile(s.dirPath+storageName+".db", nil)
	if err != nil {
		return err
	}
	vb, err := json.Marshal(v.Value)
	if err != nil {
		db.Close()
		return err
	}
	err = db.Put([]byte(v.Key), vb, nil)
	db.Close()
	return err
}

// Update will update the storage
func (s *StorageLDB) Update(storageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	v, ok := value.(*StorageType)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	k, ok := querryVaule[0].(string)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	v.Key = k
	return s.Write(storageName, value)
}

// Delete will delet the data from the storage
func (s *StorageLDB) Delete(storageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	k, ok := querryVaule[0].(string)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	s.getStorage(storageName)
	defer s.releaseStorage(storageName)
	db, err := leveldb.OpenFile(s.dirPath+storageName+".db", nil)
	if err != nil {
		return err
	}
	err = db.Delete([]byte(k), nil)
	db.Close()
	return err
}

// Read will read from the storage
func (s *StorageLDB) Read(storageName string, value interface{}, querryString string, querryVaule ...interface{}) error {
	v, ok := value.(*StorageType)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	k, ok := querryVaule[0].(string)
	if !ok {
		return fmt.Errorf("invalid data type")
	}
	s.getStorage(storageName)
	defer s.releaseStorage(storageName)
	db, err := leveldb.OpenFile(s.dirPath+storageName+".db", nil)
	if err != nil {
		return err
	}
	vb, err := db.Get([]byte(k), nil)
	db.Close()
	if err != nil {
		return err
	}
	v.Key = k
	err = json.Unmarshal(vb, &v.Value)
	return err
}

// Close will close the stroage BD
func (s *StorageLDB) Close() error {
	return nil
}
