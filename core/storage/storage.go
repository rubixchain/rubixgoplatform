package storage

const (
	StorageDBType int = iota + 1
)

type Storage interface {
	Init(storageName string, value interface{}, force bool) error
	Write(storageName string, value interface{}) error
	Update(stroageName string, value interface{}, querryString string, querryValue ...interface{}) error
	Delete(storageName string, value interface{}, querryString string, querryValue ...interface{}) error
	Read(storageName string, value interface{}, querryString string, querryValue ...interface{}) error
	WriteBatch(storageName string, value interface{}, batchSize int) error
	ReadWithOffset(storageName string, offset int, limit int, value interface{}, querryString string, querryValue ...interface{}) error
	GetDataCount(stroageName string, querryString string, querryValue ...interface{}) int64
	Drop(storageName string, value interface{}) error
	Close() error
}

type StorageType struct {
	Key   string
	Value string
}
