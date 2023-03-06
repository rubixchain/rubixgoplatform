package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/EnsurityTechnologies/config"
	"github.com/syndtr/goleveldb/leveldb"
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
	Name    string `gorm:"column:Name;primary_key;"`
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
	if err := s.Write("user", &model{Name: "TestUser2", Age: 32, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser3", Age: 34, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser1", Age: 30, Address: "Hyderabad"}); err != nil {
		var m model
		err = s.Read("user", &m, "Name=?", "TestUser1")
		if err != nil {
			t.Fatal("Failed to write storage", err.Error())
		}
		err = s.Delete("user", &model{}, "Address=?", "Hyderabad")
		if err != nil {
			t.Fatal("Failed to write storage", err.Error())
		}
		err = s.Write("user", &model{Name: "TestUser1", Age: 30, Address: "Hyderabad"})
		if err != nil {
			t.Fatal("Failed to write storage", err.Error())
		}
	}
	if err := s.Write("user", &model{Name: "TestUser2", Age: 31, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to write storage", err.Error())
	}
	if err := s.Write("user", &model{Name: "TestUser3", Age: 33, Address: "Hyderabad"}); err != nil {
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

func TestTemp(t *testing.T) {
	ts := make([]int, 0)
	ts = append(ts, 10)
	ts = append(ts, 2)
	ts = append(ts, 25)
	ts = append(ts, 50)
	ts = append(ts, -1)
	ts = append(ts, 7)
	ts = append(ts, 22)
	ts = append(ts, 40)
	jb, err := json.Marshal(ts)
	if err != nil {
		t.Fatal("failed")
	}
	var tr []int
	err = json.Unmarshal(jb, &tr)
	if err != nil {
		t.Fatal("failed")
	}
	for _, ti := range tr {
		fmt.Println(ti)
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
	str := "rf-12093738jfkfigug"
	rb := []byte(str)
	newStr := string(rb[0:2])
	if newStr == "rf" {
		fmt.Println("success")
	}

	// db, err := leveldb.OpenFile("tempdb", nil)
	// if err != nil {
	// 	t.Fatal("Failed to open db")
	// }

	// tb := make([]byte, 8096)
	// for i := range tb {
	// 	tb[i] = byte(i)
	// }
	// st := time.Now()
	// for i := 0; i < 1000000; i++ {
	// 	str := fmt.Sprintf("%d", i)
	// 	err = db.Put([]byte(str), tb, &opt.WriteOptions{Sync: true})
	// 	if err != nil {
	// 		t.Fatal("Failed to write db")
	// 	}
	// }
	// et := time.Now()
	// dif := et.Sub(st)
	// fmt.Printf("Different %v", dif)

	// db.Close()
	// os.RemoveAll("tempdb")

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
