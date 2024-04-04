package storage

const (
	StorageDBType int = iota + 1
)

type Storage interface {
	Init(storageName string, vaule interface{}, force bool) error
	Write(storageName string, vaule interface{}) error
	Update(stroageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	Delete(storageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	Read(storageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	WriteBatch(storageName string, vaule interface{}, batchSize int) error
	ReadWithOffset(storageName string, offset int, limit int, vaule interface{}, querryString string, querryVaule ...interface{}) error
	GetDataCount(stroageName string, querryString string, querryVaule ...interface{}) int64
	Close() error
}

type StorageType struct {
	Key   string
	Value string
}
