package adapter

import (
	"fmt"
	"net/url"

	"github.com/rubixchain/rubixgoplatform/wrapper/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	postgressDB string = "PostgressSQL"
	sqlDB       string = "SQLServer"
	mysqlDB     string = "MySQL"
	sqlite3     string = "Sqlite3"
)

// TenantIDStr ...
const TenantIDStr string = "TenantId"

// Adapter structer
type Adapter struct {
	db     *gorm.DB
	dbType string
}

// NewAdapter create new adapter
func NewAdapter(cfg *config.Config) (*Adapter, error) {

	var db *gorm.DB
	var err error
	switch cfg.DBType {
	case sqlDB:
		userPwd := url.UserPassword(cfg.DBUserName, cfg.DBPassword)
		dsn := fmt.Sprintf("sqlserver://%s@%s:%s?database=%s", userPwd, cfg.DBAddress, cfg.DBPort, cfg.DBName)
		db, err = gorm.Open(sqlserver.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	case postgressDB:
		dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable", cfg.DBAddress, cfg.DBPort, cfg.DBUserName, cfg.DBName, cfg.DBPassword)
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	case sqlite3:
		db, err = gorm.Open(sqlite.Open(cfg.DBAddress), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	default:
		dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", cfg.DBUserName, cfg.DBPassword, cfg.DBAddress, cfg.DBPort, cfg.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	}

	if err != nil {
		fmt.Printf("DB Adpater Error : %v", err)
		return nil, err
	}
	adapter := &Adapter{
		db:     db,
		dbType: cfg.DBType,
	}

	return adapter, err
}

func (adapter *Adapter) GetDB() *gorm.DB {
	return adapter.db
}

// InitTable Initialize table
func (adapter *Adapter) InitTable(tableName string, item interface{}, force bool) error {
	m := adapter.db.Table(tableName).Migrator()
	if force {
		return m.AutoMigrate(item)
	} else {
		if !m.HasTable(item) {
			return m.AutoMigrate(item)
		}
		return nil
	}
}

// InitTable Initialize table
func (adapter *Adapter) InitTwoTable(tableName string, item1 interface{}, item2 interface{}) error {
	err := adapter.db.Table(tableName).Migrator().AutoMigrate(item1, item2)
	return err
}

// DropTable drop the table
func (adapter *Adapter) DropTable(tableName string, item interface{}) error {
	err := adapter.db.Table(tableName).Migrator().DropTable(item)
	return err
}

// // IsTableExist check whether table exist
// func (adapter *Adapter) IsTableExist(tableName string) bool {
// 	status := adapter.db.Table(tableName).
// 	return status
// }

// DropTable drop the table
func (adapter *Adapter) AddForienKey(tableName string, value interface{}, colStr string, tableStr string) error {
	// err := adapter.db.Table(tableName).Model(value).AddForeignKey(colStr, tableStr, "CASCADE", "CASCADE").Error
	// return err
	return nil
}

// Delete function delete entry from the table
func (adapter *Adapter) Delete(tenantID interface{}, tableName string, format string, value interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := TenantIDStr + "=? AND " + format
		err := adapter.db.Table(tableName).Where(formatStr, tenantID, value).Delete(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value).Delete(item).Error
		return err
	}
}

// DeleteNew function delete entry from the table
func (adapter *Adapter) DeleteNew(tenantID interface{}, tableName string, format string, item interface{}, value ...interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		err := adapter.db.Table(tableName).Where(formatStr, value...).Delete(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value...).Delete(item).Error
		return err
	}
}

// Create creates and stores the new item in the table
func (adapter *Adapter) Create(tableName string, item interface{}) error {
	err := adapter.db.Table(tableName).Create(item).Error
	return err
}

// Create creates and stores the new item in the table
func (adapter *Adapter) CreateInBatches(tableName string, item interface{}, batchSize int) error {
	err := adapter.db.Table(tableName).CreateInBatches(item, batchSize).Error
	return err
}

// Find function finds the value from the table
func (adapter *Adapter) Find(tenantID interface{}, tableName string, format string, value interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := TenantIDStr + "=? AND " + format
		err := adapter.db.Table(tableName).Where(formatStr, tenantID, value).Find(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value).Find(item).Error
		return err
	}
}

// Find function finds the value from the table
func (adapter *Adapter) FindNew(tenantID interface{}, tableName string, format string, item interface{}, value ...interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		result := adapter.db.Table(tableName).Where(formatStr, value...).Find(item)
		if result.Error == nil {
			if result.RowsAffected == 0 {
				return fmt.Errorf("no records found")
			}
		}
		return result.Error
	} else {
		result := adapter.db.Table(tableName).Where(format, value...).Find(item)
		if result.Error == nil {
			if result.RowsAffected == 0 {
				return fmt.Errorf("no records found")
			}
		}
		return result.Error
	}
}

func (adapter *Adapter) FindWithOffset(tenantID interface{}, tableName string, format string, item interface{}, offset int, limit int, value ...interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		result := adapter.db.Table(tableName).Where(formatStr, value...).Offset(offset).Limit(limit).Find(item)
		if result.Error == nil {
			if result.RowsAffected == 0 {
				return fmt.Errorf("no records found")
			}
		}
		return result.Error
	} else {
		result := adapter.db.Table(tableName).Where(format, value...).Offset(offset).Limit(limit).Find(item)
		if result.Error == nil {
			if result.RowsAffected == 0 {
				return fmt.Errorf("no records found")
			}
		}
		return result.Error
	}
}

// FindMult function finds the value from the table
func (adapter *Adapter) FindMult(tenantID interface{}, tableName string, format1 string, format2 string, value1 interface{}, value2 interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr1 := TenantIDStr + "=? AND " + format1
		formatStr2 := TenantIDStr + "=? AND " + format2
		err := adapter.db.Table(tableName).Where(formatStr1, tenantID, value1).Or(formatStr2, tenantID, value2).Find(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format1, value1).Or(format2, value2).Find(item).Error
		return err
	}
}

// FindAnd function finds the value from the table
func (adapter *Adapter) FindAnd(tenantID interface{}, tableName string, format1 string, format2 string, value1 interface{}, value2 interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr1 := TenantIDStr + "=? AND " + format1
		formatStr2 := TenantIDStr + "=? AND " + format2
		err := adapter.db.Table(tableName).Where(formatStr1, tenantID, value1).Where(formatStr2, tenantID, value2).Find(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format1, value1).Or(format2, value2).Find(item).Error
		return err
	}
}

// FindA function finds the value from the table
func (adapter *Adapter) FindA(tenantID interface{}, tableName string, format string, value interface{}, item interface{}, item1 interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := TenantIDStr + "=? AND " + format
		err := adapter.db.Table(tableName).Where(formatStr, tenantID, value).Find(item).Association("UserId").Find(item1)
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value).Find(item).Error
		return err
	}
}

// Updates function updates the value in the table
func (adapter *Adapter) Updates(tenantID interface{}, tableName string, format string, value interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := TenantIDStr + "=? AND " + format
		err := adapter.db.Table(tableName).Where(formatStr, tenantID, value).Updates(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value).Updates(item).Error
		return err
	}
}

// UpdateNew function updates the value in the table
func (adapter *Adapter) UpdateNew(tenantID interface{}, tableName string, format string, item interface{}, value ...interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		err := adapter.db.Table(tableName).Where(formatStr, value...).Updates(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value...).Updates(item).Error
		return err
	}
}

// Save function save all the value in the table
func (adapter *Adapter) Save(tenantID interface{}, tableName string, format string, value interface{}, item interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := TenantIDStr + "=? AND " + format
		err := adapter.db.Table(tableName).Where(formatStr, tenantID, value).Save(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value).Save(item).Error
		return err
	}
}

// Save function save all the value in the table
func (adapter *Adapter) SaveNew(tenantID interface{}, tableName string, format string, item interface{}, value ...interface{}) error {
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		err := adapter.db.Table(tableName).Where(formatStr, value...).Save(item).Error
		return err
	} else {
		err := adapter.db.Table(tableName).Where(format, value...).Save(item).Error
		return err
	}
}

func (adapter *Adapter) GetCount(tenantID interface{}, tableName string, format string, value ...interface{}) int64 {
	var count int64
	var err error
	if tenantID != uuid.Nil {
		formatStr := format + " AND " + TenantIDStr + " =?"
		value = append(value, tenantID)
		err = adapter.db.Table(tableName).Where(formatStr, value...).Count(&count).Error

	} else {
		err = adapter.db.Table(tableName).Where(format, value...).Count(&count).Error
	}
	if err != nil {
		return 0
	} else {
		return count
	}
}
