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

const (
	RBTTokensType int = iota
	SmartContractTokensType
	NFTTokensType
	FTTokensType
)

type ReceivedBlockHash struct {
	BlockHash   string    `gorm:"column:block_hash;primaryKey"`
	TokenID     string    `gorm:"column:token_id"`     //one of the token from the block will be added to fullnode block hash table,
	BlockHeight uint64    `gorm:"column:block_height"` //this will be of same token which got added
	TokenValue  float64   `gorm:"column:token_value"`  //this will be of same token which got added
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	AssetType   int       `gorm:"column:asset_type"`
}

// type FullNodeSqliteTableData struct {
// 	RBTTableData      SyncedRBT           `json:"rbt_table_data"`
// 	NFTTableData      SyncedNFT           `json:"nft_table_data"`
// 	FTTableData       SyncedFT            `json:"ft_table_data"`
// 	SmartContractData SyncedSmartContract `json:"smart_contract_table_data"`
// }

type TokenProperties struct {
	TokenType         int     `json:"token_type"`
	TokenValue        float64 `json:"token_value"`
	LatestBlockHeight uint64  `json:"latest_block_height"`
	CreatorDID        string  `json:"creator_did"`
}

// type FullNodePostgresContentTablesData struct {
// 	RBTContentData RBTContent           `json:"rbt_content_data"`
// 	NFTContentData NFTContent           `json:"nft_content_data"`
// 	FTContentData  FTContent            `json:"ft_content_data"`
// 	SCContentData  SmartContractContent `json:"sc_content_data"`
// }

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

// func (w *Wallet) installTriggers(execFn func(query string) error) error {
// 	triggerInsert := `
//     CREATE TRIGGER IF NOT EXISTS trg_rbt_after_insert
//     AFTER INSERT ON FullNodeRBTTable
//     FOR EACH ROW
//     BEGIN
//         INSERT INTO FullNodeBlockHashTable (
//             block_hash,
//             token_id,
//             block_height,
// 			created_at
//         ) VALUES (
//             NEW.block_hash,
//             NEW.token_id,
//             NEW.block_height,
// 			datetime('now')
//         );
//     END;`

// 	triggerUpdate := `
// 	CREATE TRIGGER IF NOT EXISTS trg_rbt_after_update
// 	AFTER UPDATE ON FullNodeRBTTable
// 	FOR EACH ROW
// 	WHEN NEW.block_hash <> OLD.block_hash
// 	BEGIN
// 		INSERT INTO FullNodeBlockHashTable (
// 			block_hash,
// 			token_id,
// 			block_height,
// 			created_at
// 		) VALUES (
// 			NEW.block_hash,
// 			NEW.token_id,
// 			NEW.block_height,
// 			datetime('now')
// 		);
// 	END;`

// 	if err := execFn(triggerInsert); err != nil {
// 		return fmt.Errorf("failed creating insert trigger: %w", err)
// 	}
// 	if err := execFn(triggerUpdate); err != nil {
// 		return fmt.Errorf("failed creating update trigger: %w", err)
// 	}

//		w.log.Info("SQLite triggers installed successfully")
//		return nil
//	}
func (w *Wallet) installTriggers(execFn func(query string) error) error {

	type trigSpec struct {
		table         string
		assetType     int
		idColumn      string // <-- NEW: lets us customize the ID column
	}

	specs := []trigSpec{
		{"FullNodeRBTTable", RBTTokensType, "token_id"},
		{"FullNodeSCTable", SmartContractTokensType, "smart_contract_hash"}, // <-- FIX
		{"FullNodeNFTTable", NFTTokensType, "token_id"},
		{"FullNodeFTTable", FTTokensType, "token_id"},
	}

	for _, s := range specs {

		// INSERT TRIGGER
		triggerInsert := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS trg_%s_after_insert
		AFTER INSERT ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO FullNodeBlockHashTable (
				block_hash,
				token_id,
				block_height,
				token_value,
				created_at,
				asset_type
			) VALUES (
				NEW.block_hash,
				NEW.%s,
				NEW.block_height,
				NEW.token_value,
				datetime('now'),
				%d
			);
		END;`, s.table, s.table, s.idColumn, s.assetType)

		if err := execFn(triggerInsert); err != nil {
			return fmt.Errorf("failed creating insert trigger for %s: %w", s.table, err)
		}

		// UPDATE TRIGGER (only when block_hash changes)
		triggerUpdate := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS trg_%s_after_update
		AFTER UPDATE ON %s
		FOR EACH ROW
		WHEN NEW.block_hash <> OLD.block_hash
		BEGIN
			INSERT INTO FullNodeBlockHashTable (
				block_hash,
				token_id,
				block_height,
				token_value,
				created_at,
				asset_type
			) VALUES (
				NEW.block_hash,
				NEW.%s,
				NEW.block_height,
				NEW.token_value,
				datetime('now'),
				%d
			);
		END;`, s.table, s.table, s.idColumn, s.assetType)

		if err := execFn(triggerUpdate); err != nil {
			return fmt.Errorf("failed creating update trigger for %s: %w", s.table, err)
		}
	}

	w.log.Info("SQLite triggers for all token tables installed successfully")
	return nil
}


// BuildMerkleFromHashes constructs MerkleComparisonTree with Levels:
// Level 0 = root, Level N = leaves (leaves at last index)
func (w *Wallet) BuildMerkleFromHashes(hashes []string) *MerkleComparisonTree {
	if len(hashes) == 0 {
		return &MerkleComparisonTree{Levels: [][]string{{}}}
	}
	// ensure deterministic order (caller should already have sorted)
	// make a copy so we don't mutate caller slice
	cur := make([]string, len(hashes))
	copy(cur, hashes)

	levels := [][]string{cur} // leaves at index 0 for now
	// build parents until single root
	for len(cur) > 1 {
		next := []string{}
		for i := 0; i < len(cur); i += 4 {
			c1 := safeIndex(cur, i)
			c2 := safeIndex(cur, i+1)
			c3 := safeIndex(cur, i+2)
			c4 := safeIndex(cur, i+3)
			p := hashNode(c1, c2, c3, c4)
			next = append(next, p)
		}
		levels = append(levels, next)
		cur = next
	}
	// currently levels: [leaves, parents, ..., root]. Reverse to make Level0=root
	for i, j := 0, len(levels)-1; i < j; i, j = i+1, j-1 {
		levels[i], levels[j] = levels[j], levels[i]
	}
	return &MerkleComparisonTree{Levels: levels}
}

func (w *Wallet) BuildMerkleStructForComparison(epoch string) *MerkleComparisonTree {
	// 1️⃣ Fetch block hashes
	records, err := w.ReadBlocksForEpoch(epoch)
	if err != nil || len(records) == 0 {
		return &MerkleComparisonTree{Levels: [][]string{{""}}} // empty tree with 1-level placeholder
	}

	// 2️⃣ Extract and sort (deterministic ordering)
	hashes := make([]string, len(records))
	for i, r := range records {
		hashes[i] = r.BlockHash
	}
	sort.Strings(hashes)

	return w.BuildMerkleFromHashes(hashes)
	// // 3️⃣ Start tree with leaf level
	// levels := [][]string{hashes} // leaves temporarily stored at index 0

	// // 4️⃣ Generate parent layers until root
	// current := hashes
	// for len(current) > 1 {
	// 	next := []string{}

	// 	for i := 0; i < len(current); i += 4 {
	// 		// up to 4 children per parent
	// 		c1 := safeIndex(current, i)
	// 		c2 := safeIndex(current, i+1)
	// 		c3 := safeIndex(current, i+2)
	// 		c4 := safeIndex(current, i+3)

	// 		parentHash := hashNode(c1, c2, c3, c4)
	// 		next = append(next, parentHash)
	// 	}

	// 	levels = append(levels, next)
	// 	current = next
	// }

	// // 5️⃣ Reverse the slice so the final ordering is:
	// //     Level 0 = root
	// //     ...
	// //     Last level = leaf hashes
	// for i, j := 0, len(levels)-1; i < j; i, j = i+1, j-1 {
	// 	levels[i], levels[j] = levels[j], levels[i]
	// }

	// return &MerkleComparisonTree{Levels: levels}
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
	if idx < 0 || idx >= len(arr) {
		return "" // padding value
	}
	return arr[idx]
}

// Root returns the top of the Merkle tree
func (t *MerkleComparisonTree) Root() string {
	if len(t.Levels) == 0 || len(t.Levels[0]) == 0 {
		return ""
	}
	return t.Levels[0][0]
}
func (t *MerkleComparisonTree) Height() int {
	return len(t.Levels)
}

// GetChildren returns up to 4 child hashes of a node
func (t *MerkleComparisonTree) GetChildren(level, index int) []string {
	// If this is already a leaf level, it has no children.
	if level >= len(t.Levels)-1 {
		return []string{"", "", "", ""}
	}

	childrenLevel := level + 1
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
	// Leaf layer is ALWAYS the LAST level in the tree
	return level == len(t.Levels)-1
}
