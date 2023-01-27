package storage

import (
	"os"
	"testing"

	"github.com/EnsurityTechnologies/config"
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
