package core

import (
	"github.com/rubixchain/rubixgoplatform/core/storage"
)

// TrackedStorage wraps storage operations with performance tracking
type TrackedStorage struct {
	storage storage.RubixDB
	c       *Core
}

// NewTrackedStorage creates a new tracked storage wrapper
func NewTrackedStorage(rubixDB storage.RubixDB, c *Core) *TrackedStorage {
	return &TrackedStorage{
		storage: rubixDB,
		c:       c,
	}
}

func (t *TrackedStorage) Close() error {
	defer t.c.TrackOperation("db.close", nil)(nil)
	t.storage.Close()
	return nil
}
