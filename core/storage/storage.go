package storage

type Storage interface {
	Init(storageName string, vaule interface{}) error
	Write(storageName string, vaule interface{}) error
	Update(stroageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	Delete(storageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	Read(storageName string, vaule interface{}, querryString string, querryVaule ...interface{}) error
	Close() error
}
