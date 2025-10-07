package core

import (
	"time"

	"github.com/rubixchain/rubixgoplatform/core/storage"
)

// TrackedStorage wraps storage operations with performance tracking
type TrackedStorage struct {
	storage storage.Storage
	c       *Core
}

// NewTrackedStorage creates a new tracked storage wrapper
func NewTrackedStorage(s storage.Storage, c *Core) *TrackedStorage {
	return &TrackedStorage{
		storage: s,
		c:       c,
	}
}

func (t *TrackedStorage) Init(storageName string, value interface{}, force bool) error {
	defer t.c.TrackOperation("db.init", map[string]interface{}{
		"storage_name": storageName,
		"force":        force,
	})(nil)
	
	return t.storage.Init(storageName, value, force)
}

func (t *TrackedStorage) Write(storageName string, value interface{}) error {
	defer t.c.TrackOperation("db.write", map[string]interface{}{
		"storage_name": storageName,
	})(nil)
	
	return t.storage.Write(storageName, value)
}

func (t *TrackedStorage) Update(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	defer t.c.TrackOperation("db.update", map[string]interface{}{
		"storage_name": storageName,
		"query":        queryString,
	})(nil)
	
	return t.storage.Update(storageName, value, queryString, queryValue...)
}

func (t *TrackedStorage) Delete(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	defer t.c.TrackOperation("db.delete", map[string]interface{}{
		"storage_name": storageName,
		"query":        queryString,
	})(nil)
	
	return t.storage.Delete(storageName, value, queryString, queryValue...)
}

func (t *TrackedStorage) Read(storageName string, value interface{}, queryString string, queryValue ...interface{}) error {
	defer t.c.TrackOperation("db.read", map[string]interface{}{
		"storage_name": storageName,
		"query":        queryString,
	})(nil)
	
	return t.storage.Read(storageName, value, queryString, queryValue...)
}

func (t *TrackedStorage) WriteBatch(storageName string, value interface{}, batchSize int) error {
	start := time.Now()
	
	// Get count of items if possible
	var itemCount int
	switch v := value.(type) {
	case []interface{}:
		itemCount = len(v)
	}
	
	err := t.storage.WriteBatch(storageName, value, batchSize)
	
	t.c.TrackOperation("db.write_batch", map[string]interface{}{
		"storage_name": storageName,
		"batch_size":   batchSize,
		"item_count":   itemCount,
		"duration_ms":  time.Since(start).Milliseconds(),
	})(err)
	
	return err
}

func (t *TrackedStorage) ReadWithOffset(storageName string, offset int, limit int, value interface{}, queryString string, queryValue ...interface{}) error {
	defer t.c.TrackOperation("db.read_with_offset", map[string]interface{}{
		"storage_name": storageName,
		"offset":       offset,
		"limit":        limit,
		"query":        queryString,
	})(nil)
	
	return t.storage.ReadWithOffset(storageName, offset, limit, value, queryString, queryValue...)
}

func (t *TrackedStorage) GetDataCount(storageName string, queryString string, queryValue ...interface{}) int64 {
	start := time.Now()
	count := t.storage.GetDataCount(storageName, queryString, queryValue...)
	
	t.c.TrackOperation("db.get_data_count", map[string]interface{}{
		"storage_name": storageName,
		"query":        queryString,
		"count":        count,
		"duration_ms":  time.Since(start).Milliseconds(),
	})(nil)
	
	return count
}

func (t *TrackedStorage) Drop(storageName string, value interface{}) error {
	defer t.c.TrackOperation("db.drop", map[string]interface{}{
		"storage_name": storageName,
	})(nil)
	
	return t.storage.Drop(storageName, value)
}

func (t *TrackedStorage) Close() error {
	defer t.c.TrackOperation("db.close", nil)(nil)
	return t.storage.Close()
}