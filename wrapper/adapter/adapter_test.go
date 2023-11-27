package adapter

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type model struct {
	ID      int    `gorm:"column:Id;primary_key;auto_increment"`
	Name    string `gorm:"column:Name;not null"`
	Age     int    `gorm:"column:Age"`
	Address string `gorm:"column:Address"`
}

func TestBasic(t *testing.T) {
	rb, err := ioutil.ReadFile("config.json")
	if err != nil {
		t.Fatal("Failed to read config file")
	}
	var cfg config.Config

	err = json.Unmarshal(rb, &cfg)
	if err != nil {
		t.Fatal("Failed to parse config file")
	}
	ad, err := NewAdapter(&cfg)
	if err != nil {
		t.Fatal("Failed to initialize adapter")
	}

	if err := ad.InitTable("user", &model{}, true); err != nil {
		t.Fatal("Failed to initialize table", err.Error())
	}
	//defer ad.DropTable("user", &model{})
	var rm model
	err = ad.FindNew(uuid.Nil, "user", "Name=?", &rm, "TestUser1")
	if err == nil {
		t.Fatal("DB should no thave data")
	}
	if err := ad.Create("user", &model{Name: "TestUser1", Age: 32, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to create data", err.Error())
	}
	if err := ad.Create("user", &model{Name: "TestUser2", Age: 35, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to create data", err.Error())
	}
	if err := ad.Create("user", &model{Name: "TestUser3", Age: 40, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to create data", err.Error())
	}
	if err := ad.Create("user", &model{Name: "TestUser4", Age: 45, Address: "Hyderabad"}); err != nil {
		t.Fatal("Failed to create data", err.Error())
	}
	ud := make([]model, 10)
	for i := range ud {
		ud[i].Name = fmt.Sprintf("User%d", i)
		ud[i].Age = 45 + i
		ud[i].Address = "Hyderabad"
	}
	if err := ad.CreateInBatches("user", ud, 10); err != nil {
		t.Fatal("Failed to write data", err.Error())
	}
	var m model
	if err := ad.FindNew(uuid.Nil, "user", "Name=?", &m, "TestUser1"); err != nil {
		t.Fatal("Failed to read data", err.Error())
	}
	if m.Name != "TestUser1" {
		t.Fatal("Data mismatch", err.Error())
	}
	var m1 []model
	if err := ad.FindNew(uuid.Nil, "user", "Age>? AND  Age<?", &m1, 33, 41); err != nil {
		t.Fatal("Failed to read data", err.Error())
	}
	if len(m1) != 2 {
		t.Fatal("Data mismatch")
	}
	if m1[0].Name != "TestUser2" || m1[1].Name != "TestUser3" {
		t.Fatal("Data mismatch")
	}
	db, err := ad.db.DB()
	if err != nil {
		t.Fatal("Failed to close db", err.Error())
	}
	if err := db.Close(); err != nil {
		t.Fatal("Failed to close db", err.Error())
	}
}
