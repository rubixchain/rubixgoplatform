package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/rubixchain/rubixgoplatform/core/storage"
	gormv2 "gorm.io/gorm"
)

type ReceivedBlockHash struct {
	BlockHash   string    `gorm:"column:block_hash;primaryKey"`
	TokenID     string    `gorm:"column:token_id"`
	BlockHeight uint64    `gorm:"column:block_height"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

// MerkleComparisonTree holds all nodes of the arity-4 Merkle tree in memory
// for fast lookup during reconciliation.
type MerkleComparisonTree struct {
	Levels [][]string // Levels[level][index] → hash
}

func (w *Wallet) setupTriggers() error {
	raw := w.fullNodeSQLDB.(*storage.StorageDB).RawDB()

	// --- GORM v2 (gorm.io/gorm) ---
	if db2, ok := raw.(*gormv2.DB); ok {
		if db2.Config.Dialector.Name() != "sqlite" {
			w.log.Warn("Skipping trigger setup (DB is not SQLite)")
			return nil
		}

		return w.installTriggers(func(q string) error {
			return db2.Exec(q).Error
		})
	}

	// --- GORM v1 (github.com/jinzhu/gorm) ---
	if db1, ok := raw.(*gorm.DB); ok {
		if db1.Dialect().GetName() != "sqlite3" {
			w.log.Warn("Skipping trigger setup (DB is not SQLite3)")
			return nil
		}

		return w.installTriggers(func(q string) error {
			return db1.Exec(q).Error
		})
	}

	w.log.Warn("Skipping trigger setup (unknown DB type)")
	return nil
}

func (w *Wallet) installTriggers(execFn func(query string) error) error {
	triggerInsert := `
    CREATE TRIGGER IF NOT EXISTS trg_rbt_after_insert
    AFTER INSERT ON FullNodeRBTTable
    FOR EACH ROW
    BEGIN
        INSERT INTO FullNodeBlockHashTable (
            block_hash,
            token_id,
            block_height,
			created_at
        ) VALUES (
            NEW.block_hash,
            NEW.token_id,
            NEW.block_height,
			datetime('now')
        );
    END;`

	triggerUpdate := `
	CREATE TRIGGER IF NOT EXISTS trg_rbt_after_update
	AFTER UPDATE ON FullNodeRBTTable
	FOR EACH ROW
	WHEN NEW.block_hash <> OLD.block_hash
	BEGIN
		INSERT INTO FullNodeBlockHashTable (
			block_hash,
			token_id,
			block_height,
			created_at
		) VALUES (
			NEW.block_hash,
			NEW.token_id,
			NEW.block_height,
			datetime('now')
		);
	END;`

	if err := execFn(triggerInsert); err != nil {
		return fmt.Errorf("failed creating insert trigger: %w", err)
	}
	if err := execFn(triggerUpdate); err != nil {
		return fmt.Errorf("failed creating update trigger: %w", err)
	}

	w.log.Info("SQLite triggers installed successfully")
	return nil
}
func (w *Wallet) BuildMerkleStructForComparison(epoch string) *MerkleComparisonTree {
	// 1️⃣ Fetch block hashes
	records, err := w.ReadBlocksForEpoch(epoch)
	if err != nil || len(records) == 0 {
		return &MerkleComparisonTree{Levels: [][]string{{}}} // empty tree
	}

	// 2️⃣ Extract and sort deterministically (important!)
	hashes := make([]string, len(records))
	for i, r := range records {
		hashes[i] = r.BlockHash
	}
	sort.Strings(hashes)

	// 3️⃣ Build the leaf level
	tree := &MerkleComparisonTree{}
	currentLevel := hashes
	tree.Levels = append(tree.Levels, currentLevel)

	// 4️⃣ Build internal levels upward until root
	for len(currentLevel) > 1 {
		nextLevel := []string{}

		for i := 0; i < len(currentLevel); i += 4 {
			// collect up to 4 children
			c1 := currentLevel[i]
			c2 := safeIndex(currentLevel, i+1)
			c3 := safeIndex(currentLevel, i+2)
			c4 := safeIndex(currentLevel, i+3)

			parentHash := hashNode(c1, c2, c3, c4)
			nextLevel = append(nextLevel, parentHash)
		}

		tree.Levels = append(tree.Levels, nextLevel)
		currentLevel = nextLevel
	}

	return tree
}

func hashNode(values ...string) string {
	h := sha256.New()
	for _, v := range values {
		h.Write([]byte(v))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// helper to return deterministic padding when index > length
func safeIndex(arr []string, idx int) string {
	if idx >= len(arr) {
		return "0" // padding hash
	}
	return arr[idx]
}

// Root returns the top of the Merkle tree
func (t *MerkleComparisonTree) Root() string {
	last := len(t.Levels) - 1
	return t.Levels[last][0]
}

// GetChildren returns up to 4 child hashes of a node
func (t *MerkleComparisonTree) GetChildren(level, index int) []string {
	// children always belong to previous level (because root is last)
	if level == 0 {
		// root level has no children — this case is handled earlier
		return []string{"", "", "", ""}
	}

	childrenLevel := level - 1
	start := index * 4

	return []string{
		safeIndex(t.Levels[childrenLevel], start),
		safeIndex(t.Levels[childrenLevel], start+1),
		safeIndex(t.Levels[childrenLevel], start+2),
		safeIndex(t.Levels[childrenLevel], start+3),
	}
}

// IsLeaf returns true if the next level would be the leaf layer
func (t *MerkleComparisonTree) IsLeaf(level int) bool {
	return level == 0
}