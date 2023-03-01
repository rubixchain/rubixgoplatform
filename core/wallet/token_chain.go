package wallet

import (
	"fmt"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	TokenStatusStorage string = "TokenStatus"
)

const (
	WholeTokenType string = "wt"
	PartTokenType  string = "pt"
	NFTType        string = "nft"
	DataTokenType  string = "dt"
)

const TCBlockCountLimit int = 100

func tcsPrefix(tokenType string, token string) string {
	return tokenType + "-" + token + "-"
}

func tcsKey(tokenType string, token string, blockID string) string {
	return tokenType + "-" + token + "-" + blockID
}

func tcsBlockID(token string, key string) string {
	if strings.HasPrefix(key, token+"-") {
		return strings.Trim(key, token+"-")
	} else {
		return ""
	}
}

func (w *Wallet) getChainDB(tt string) *ChainDB {
	var db *ChainDB
	switch tt {
	case WholeTokenType:
		db = w.tcs
	case PartTokenType:
		db = w.tcs
	case DataTokenType:
		db = w.dtcs
	}
	return db
}

// getBlock get chain block from the storage
func (w *Wallet) getBlock(tt string, t string, blockID string) ([]byte, error) {
	db := w.getChainDB(tt)
	if db == nil {
		return nil, fmt.Errorf("failed get block, invalid token type")
	}
	v, err := db.Get([]byte(tcsKey(tt, t, blockID)), nil)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// getAllBlocks get the chain blocks
func (w *Wallet) getAllBlocks(tt string, token string, blockID string) ([][]byte, string, error) {
	db := w.getChainDB(tt)
	if db == nil {
		return nil, "", fmt.Errorf("failed get all blocks, invalid token type")
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	blks := make([][]byte, 0)
	count := 0
	if blockID != "" {
		if !iter.Seek([]byte(tcsKey(tt, token, blockID))) {
			return nil, "", fmt.Errorf("Token chain block does not exist")
		}
	}
	nextBlkID := ""
	for iter.Next() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		blks = append(blks, blk)
		count++
		if count == TCBlockCountLimit {
			b := block.InitBlock(blk, nil)
			blkID, err := b.GetBlockID(token)
			if err != nil {
				return nil, "", fmt.Errorf("Invalid token chain block")
			}
			nextBlkID = blkID
		}
	}
	return blks, nextBlkID, nil
}

// getLatestBlock get latest block from the storage
func (w *Wallet) getLatestBlock(tt string, token string) *block.Block {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get latest block, invalid token type")
		return nil
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	if iter.Last() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		b := block.InitBlock(blk, nil)
		return b
	}
	return nil
}

// getFirstBlock get the first block from the storage
func (w *Wallet) getFirstBlock(tt string, token string) *block.Block {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get first block, invalid token type")
		return nil
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	if iter.First() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		b := block.InitBlock(blk, nil)
		return b
	}
	return nil
}

// addBlock will write block into storage
func (w *Wallet) addBlock(tt string, token string, b *block.Block) error {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to add block, invalid token type")
		return nil
	}
	bid, err := b.GetBlockID(token)
	if err != nil {
		return err
	}
	key := tcsKey(tt, token, bid)
	lb := w.getLatestBlock(tt, token)
	bn, err := b.GetBlockNumber(token)
	if err != nil {
		w.log.Error("Failed to get block number", "err", err)
		return err
	}
	// First block check block number start with zero
	if lb == nil {
		if bn != 0 {
			w.log.Error("Invalid block number, expect 0", "bn", bn)
			return fmt.Errorf("invalid block number")
		}
	} else {
		lbn, err := lb.GetBlockNumber(token)
		if err != nil {
			w.log.Error("Failed to get block number", "err", err)
			return err
		}
		if lbn+1 != bn {
			w.log.Error("Invalid block number, sequence missing", "lbn", lbn, "bn", bn)
			return fmt.Errorf("invalid block number, sequence missing")
		}
	}
	db.l.Lock()
	err = db.Put([]byte(key), b.GetBlock(), nil)
	db.l.Unlock()
	return err
}
