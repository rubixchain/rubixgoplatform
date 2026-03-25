package storage

// Legacy SQLite-style CRUD stubs on RubixDB.
// These satisfy TrackedStorage which wraps the old storage interface.
// TODO(phase07): remove TrackedStorage and these stubs when the legacy wrapper is deleted.

func (r *RubixDB) Init(storageName string, value interface{}, force bool) error {
	return nil
}

func (r *RubixDB) Write(storageName string, value interface{}) error {
	return nil
}

func (r *RubixDB) Update(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	return nil
}

func (r *RubixDB) Delete(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	return nil
}

func (r *RubixDB) Read(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	return nil
}

func (r *RubixDB) WriteBatch(storageName string, value interface{}, batchSize int) error {
	return nil
}

func (r *RubixDB) ReadWithOffset(storageName string, offset int, limit int, value interface{}, queryString string, queryValue ...interface{}) error {
	return nil
}

func (r *RubixDB) GetDataCount(storageName string, queryString string, queryValue ...interface{}) int64 {
	return 0
}

func (r *RubixDB) Drop(storageName string, value interface{}) error {
	return nil
}

func (r *RubixDB) UpdateColumn(storageName string, columnName string, columnValue interface{}, conditionString string, conditionValue interface{}) error {
	return nil
}
