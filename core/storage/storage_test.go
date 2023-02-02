package storage

import (
	"fmt"
	"os"
	"testing"

	"github.com/EnsurityTechnologies/config"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func initStorageDB() (*StorageDB, error) {
	cfg := &config.Config{
		DBAddress: "test.db",
		DBType:    "Sqlite3",
	}
	return NewStorageDB(cfg)
}

func remoteStorageDB() error {
	return os.Remove("test.db")
}

type model struct {
	ID      int    `gorm:"column:Id;primary_key;auto_increment"`
	Name    string `gorm:"column:Name;not null"`
	Age     int    `gorm:"column:Age"`
	Address string `gorm:"column:Address"`
}

func TestBasic(t *testing.T) {
	var s Storage
	var err error

	s, err = initStorageDB()
	if err != nil {
		t.Fatal("Failed to init DB", err.Error())
	}
	if err := s.Init("user", &model{}); err != nil {
		t.Fatal("Failed to initialize storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser1", Age: 20, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser2", Age: 30, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser3", Age: 32, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	var m model
	if err := s.Read("user", &m, "Name=?", "TestUser1"); err != nil {
		t.Fatal("Failed to get data from storage", err.Error())
	}

	if m.Name != "TestUser1" {
		t.Fatal("User name miss match")
	}

	if err := s.Close(); err != nil {
		t.Fatal("Failed to close storage", err.Error())
	}

	if err := remoteStorageDB(); err != nil {
		t.Fatal("Failed to remove storage", err.Error())
	}
}

func TestLevelLB(t *testing.T) {
	var s Storage
	var err error
	s, err = NewStorageLDB("./")
	if err != nil {
		t.Fatal("Failed to setup level db", err.Error())
	}
	if err := s.Init("Test", &StorageType{}); err != nil {
		t.Fatal("Failed to initialize storage", err.Error())
	}

	if err := s.Write("Test", &StorageType{Key: "Key1", Value: "Value1"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("Test", &StorageType{Key: "Key2", Value: "Value2"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("Test", &StorageType{Key: "Key3", Value: "Value3"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	var st StorageType
	if err := s.Read("Test", &st, "key=?", "Key1"); err != nil {
		t.Fatal("Failed to get data from storage", err.Error())
	}

	if st.Value != "Value1" {
		t.Fatal("Value miss match")
	}

	if err := s.Close(); err != nil {
		t.Fatal("Failed to close storage", err.Error())
	}
}

func TestSanp(t *testing.T) {
	db, err := leveldb.OpenFile("tempdb", nil)
	if err != nil {
		t.Fatal("Failed to open db")
	}

	db.Put([]byte("token1-entry1"), []byte("token1-entry1"), nil)
	db.Put([]byte("token1-entry2"), []byte("token1-entry2"), nil)
	db.Put([]byte("token2-entry1"), []byte("token2-entry1"), nil)
	db.Put([]byte("token1-entry3"), []byte("token1-entry3"), nil)
	db.Put([]byte("token2-entry2"), []byte("token2-entry2"), nil)
	db.Put([]byte("token2-entry3"), []byte("token2-entry3"), nil)
	db.Put([]byte("token2-entry4"), []byte("token2-entry4"), nil)
	db.Put([]byte("token1-entry4"), []byte("token1-entry4"), nil)
	db.Put([]byte("token1-entry2"), []byte("token1-entry2-updated"), nil)
	iter := db.NewIterator(util.BytesPrefix([]byte("token1-")), nil)
	if err != nil {
		t.Fatal("Failed to get sanp")
	}
	//iter.Last()
	key := iter.Key()
	value := iter.Value()
	fmt.Printf("%s : %s\n", string(key), string(value))
	iter.Seek([]byte("token1-entry3"))
	for {
		key := iter.Key()
		value := iter.Value()
		fmt.Printf("%s : %s\n", string(key), string(value))
		if !iter.Next() {
			break
		}
	}
	iter.Release()
	db.Close()
	os.RemoveAll("tempdb")

}

func TestNodeDB(t *testing.T) {

	db, err := leveldb.OpenFile("../../windows/node1/Rubix/TestNet/tokenchainstorage", nil)
	if err != nil {
		t.Fatal("Failed to open db")
	}
	iter := db.NewIterator(nil, nil)
	if err != nil {
		t.Fatal("Failed to get sanp")
	}
	for iter.Next() {
		key := iter.Key()
		//value := iter.Value()
		fmt.Printf("%s\n", string(key))
	}
	iter.Release()
	db.Close()
}
