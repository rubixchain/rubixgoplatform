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
)

const TCBlockCountLimit int = 100

const (
	TokenMintedType      string = "token_minted"
	TokenTransferredType string = "token_transferred"
	TokenMigratedType    string = "token_migrated"
	TokenPledgedType     string = "token_pledged"
	TokenGeneratedType   string = "token_generated"
)

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

// GetTokenBlock get token chain block from the storage
func (w *Wallet) GetTokenBlock(token string, blockID string) ([]byte, error) {
	v, err := w.tcs.Get([]byte(tcsKey(WholeTokenType, token, blockID)), nil)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetAllTokenBlocks get the tokecn chain blocks
func (w *Wallet) GetAllTokenBlocks(token string, blockID string) ([][]byte, string, error) {
	iter := w.tcs.NewIterator(util.BytesPrefix([]byte(tcsPrefix(WholeTokenType, token))), nil)
	defer iter.Release()
	blks := make([][]byte, 0)
	count := 0
	if blockID != "" {
		if !iter.Seek([]byte(tcsKey(WholeTokenType, token, blockID))) {
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
			b := block.InitBlock(block.TokenBlockType, blk, nil)
			blkID, err := b.GetBlockID(token)
			if err != nil {
				return nil, "", fmt.Errorf("Invalid token chain block")
			}
			nextBlkID = blkID
		}
	}
	return blks, nextBlkID, nil
}

// GetLatestTokenBlock get latest token block from the storage
func (w *Wallet) GetLatestTokenBlock(token string) *block.Block {
	iter := w.tcs.NewIterator(util.BytesPrefix([]byte(tcsPrefix(WholeTokenType, token))), nil)
	defer iter.Release()
	if iter.Last() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		b := block.InitBlock(block.TokenBlockType, blk, nil)
		return b
	}
	return nil
}

func (w *Wallet) getGensysBlock(token string) *block.Block {
	iter := w.tcs.NewIterator(util.BytesPrefix([]byte(tcsPrefix(WholeTokenType, token))), nil)
	defer iter.Release()
	if iter.First() {
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		b := block.InitBlock(block.TokenBlockType, blk, nil)
		return b
	}
	return nil
}

// AddTokenBlock will write token block into storage
func (w *Wallet) AddTokenBlock(token string, b *block.Block) error {
	bid, err := b.GetBlockID(token)
	if err != nil {
		return err
	}
	key := tcsKey(WholeTokenType, token, bid)
	lb := w.GetLatestTokenBlock(token)
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
	w.wl.Lock()
	err = w.tcs.Put([]byte(key), b.GetBlock(), nil)
	w.wl.Unlock()
	return err
}
