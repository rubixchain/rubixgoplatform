package wallet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rubixchain/rubixgoplatform/block"
	tkn "github.com/rubixchain/rubixgoplatform/token"
	ut "github.com/rubixchain/rubixgoplatform/util"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	DefaultKeyLength int = 131
)

const (
	TokenStatusStorage string = "TokenStatus"
)

const (
	WholeTokenType         string = "wt"
	PartTokenType          string = "pt"
	NFTType                string = "nt"
	TestTokenType          string = "tt"
	DataTokenType          string = "dt"
	TestPartTokenType      string = "tp"
	TestNFTType            string = "tn"
	ReferenceType          string = "rf"
	SmartContractTokenType string = "st"
)

const TCBlockCountLimit int = 100

func tcsType(tokenType int) string {
	tt := "wt"
	switch tokenType {
	case tkn.RBTTokenType:
		tt = WholeTokenType
	case tkn.PartTokenType:
		tt = PartTokenType
	case tkn.TestPartTokenType:
		tt = TestPartTokenType
	case tkn.NFTTokenType:
		tt = NFTType
	case tkn.TestNFTTokenType:
		tt = TestNFTType
	case tkn.TestTokenType:
		tt = TestTokenType
	case tkn.DataTokenType:
		tt = DataTokenType
	case tkn.SmartContractTokenType:
		tt = SmartContractTokenType
	}
	return tt + "-"
}

func tcsPrefix(tokenType int, t string) string {
	tt := "wt"
	switch tokenType {
	case tkn.RBTTokenType:
		tt = WholeTokenType
	case tkn.PartTokenType:
		tt = PartTokenType
	case tkn.TestPartTokenType:
		tt = TestPartTokenType
	case tkn.NFTTokenType:
		tt = NFTType
	case tkn.TestNFTTokenType:
		tt = TestNFTType
	case tkn.TestTokenType:
		tt = TestTokenType
	case tkn.DataTokenType:
		tt = DataTokenType
	case tkn.SmartContractTokenType:
		tt = SmartContractTokenType
	}
	return tt + "-" + t + "-"
}

func tcsKey(tokenType int, t string, blockID string) string {
	tt := "wt"
	switch tokenType {
	case tkn.RBTTokenType:
		tt = WholeTokenType
	case tkn.PartTokenType:
		tt = PartTokenType
	case tkn.TestPartTokenType:
		tt = TestPartTokenType
	case tkn.NFTTokenType:
		tt = NFTType
	case tkn.TestNFTTokenType:
		tt = TestNFTType
	case tkn.TestTokenType:
		tt = TestTokenType
	case tkn.DataTokenType:
		tt = DataTokenType
	case tkn.SmartContractTokenType:
		tt = SmartContractTokenType
	}
	bs := strings.Split(blockID, "-")
	if len(bs) == 2 {
		bn, err := strconv.ParseUint(bs[0], 10, 64)
		if err != nil {
			return tt + "-" + t + "-" + blockID
		}
		return tt + "-" + t + "-" + fmt.Sprintf("%016x", bn) + "-" + bs[1]
	}
	return tt + "-" + t + "-" + blockID
}

func old2NewKey(key string) string {
	bs := strings.Split(key, "-")
	if len(bs) == 4 {
		bn, err := strconv.ParseUint(bs[2], 10, 64)
		if err != nil {
			return key
		}
		return bs[0] + "-" + bs[1] + "-" + fmt.Sprintf("%016x", bn) + "-" + bs[3]
	}
	return key
}

func isOldKey(key string) bool {
	return len(key) != DefaultKeyLength
}

func oldtcsKey(tokenType int, t string, blockID string) string {
	tt := "wt"
	switch tokenType {
	case tkn.RBTTokenType:
		tt = WholeTokenType
	case tkn.PartTokenType:
		tt = PartTokenType
	case tkn.NFTTokenType:
		tt = NFTType
	case tkn.TestTokenType:
		tt = TestTokenType
	case tkn.DataTokenType:
		tt = DataTokenType
	case tkn.SmartContractTokenType:
		tt = SmartContractTokenType
	}
	return tt + "-" + t + "-" + blockID
}

func tcsBlockID(token string, key string) string {
	if strings.HasPrefix(key, token+"-") {
		return strings.Trim(key, token+"-")
	} else {
		return ""
	}
}

func (w *Wallet) getChainDB(tt int) *ChainDB {
	var db *ChainDB
	switch tt {
	case tkn.RBTTokenType:
		db = w.tcs
	case tkn.PartTokenType:
		db = w.tcs
	case tkn.TestTokenType:
		db = w.tcs
	case tkn.TestPartTokenType:
		db = w.tcs
	case tkn.DataTokenType:
		db = w.dtcs
	case tkn.TestDataTokenType:
		db = w.dtcs
	case tkn.NFTTokenType:
		db = w.ntcs
	case tkn.TestNFTTokenType:
		db = w.ntcs
	case tkn.SmartContractTokenType:
		db = w.smartContractTokenChainStorage
	}
	return db
}

func (w *Wallet) getRawBlock(db *ChainDB, key []byte) ([]byte, error) {
	v, err := db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	blk := make([]byte, len(v))
	copy(blk, v)
	if string(blk[0:2]) == ReferenceType {
		return w.getRawBlock(db, blk)
	} else {
		return blk, nil
	}
}

// getBlock get chain block from the storage
func (w *Wallet) getBlock(tt int, t string, blockID string) ([]byte, error) {
	db := w.getChainDB(tt)
	if db == nil {
		return nil, fmt.Errorf("failed get block, invalid token type")
	}
	return w.getRawBlock(db, []byte(tcsKey(tt, t, blockID)))
}

// getAllBlocks get the chain blocks
func (w *Wallet) getAllBlocks(tt int, token string, blockID string) ([][]byte, string, error) {
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
	var err error
	for iter.Next() {
		key := string(iter.Key())
		if isOldKey(key) {
			err = w.updateNewKey(tt, token)
			if err != nil {
				w.log.Error("Failed to update new key", "err", err)
				return nil, "", err
			}
			return w.getAllBlocks(tt, token, blockID)
		}
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		if string(blk[0:2]) == ReferenceType {
			blk, err = w.getRawBlock(db, blk)
			if err != nil {
				return nil, "", err
			}
		}
		blks = append(blks, blk)
		count++
		if count == TCBlockCountLimit {
			b := block.InitBlock(blk, nil)
			blkID, err := b.GetBlockID(token)
			if err != nil {
				return nil, "", fmt.Errorf("invalid token chain block")
			}
			nextBlkID = blkID
		}
	}
	return blks, nextBlkID, nil
}

func (w *Wallet) updateNewKey(tt int, token string) error {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get latest block, invalid token type")
		return nil
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	for iter.Next() {
		key := string(iter.Key())
		if isOldKey(key) {
			v := iter.Value()
			blk := make([]byte, len(v))
			copy(blk, v)
			db.l.Lock()
			err := db.Delete([]byte(key), nil)
			if err == nil {
				err = db.Put([]byte(old2NewKey(key)), blk, nil)
			}
			db.l.Unlock()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// getGenesisBlock get the genesis block from the storage
func (w *Wallet) getGenesisBlock(tt int, token string) *block.Block {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get first block, invalid token type")
		return nil
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	var err error
	if iter.First() {
		key := string(iter.Key())
		if isOldKey(key) {
			err = w.updateNewKey(tt, token)
			if err != nil {
				w.log.Error("Failed to update new key", "err", err)
				return nil
			}
			return w.getGenesisBlock(tt, token)
		}
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		if string(blk[0:2]) == ReferenceType {
			blk, err = w.getRawBlock(db, blk)
			if err != nil {
				return nil
			}
		}
		b := block.InitBlock(blk, nil)
		return b
	}
	return nil
}

// getLatestBlock get latest block from the storage
func (w *Wallet) getLatestBlock(tt int, token string) *block.Block {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to get latest block, invalid token type")
		return nil
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsPrefix(tt, token))), nil)
	defer iter.Release()
	var err error
	if iter.Last() {
		key := string(iter.Key())
		if isOldKey(key) {
			err = w.updateNewKey(tt, token)
			if err != nil {
				w.log.Error("Failed to update new key", "err", err)
				return nil
			}
			w.log.Debug("Keys are updated successfully")
			return w.getLatestBlock(tt, token)
		}
		v := iter.Value()
		blk := make([]byte, len(v))
		copy(blk, v)
		if string(blk[0:2]) == ReferenceType {
			blk, err = w.getRawBlock(db, blk)
			if err != nil {
				w.log.Error("Failed to get reference block", "err", err)
				return nil
			}
		}
		b := block.InitBlock(blk, nil)
		return b
	}
	return nil
}

// addBlock will write block into storage
func (w *Wallet) addBlock(token string, b *block.Block) error {
	opt := &opt.WriteOptions{
		Sync: true,
	}
	tt := b.GetTokenType(token)
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to add block, invalid token type")
		return fmt.Errorf("failed to get db")
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
	if b.CheckMultiTokenBlock() {
		bs, err := b.GetHash()
		if err != nil {
			return err
		}
		hs := ut.HexToStr(ut.CalculateHash(b.GetBlock(), "SHA3-256"))
		refkey := []byte(ReferenceType + "-" + hs + "-" + bs)
		_, err = w.getRawBlock(db, refkey)
		// Write only if reference block not exist
		if err != nil {
			db.l.Lock()
			err = db.Put(refkey, b.GetBlock(), opt)
			db.l.Unlock()
			if err != nil {
				return err
			}
		}
		db.l.Lock()
		err = db.Put([]byte(key), refkey, opt)
		db.l.Unlock()
		return err
	} else {
		db.l.Lock()
		err = db.Put([]byte(key), b.GetBlock(), opt)
		if tt == tkn.TestTokenType {
			w.log.Debug("Writtent", "key", key)
		}
		db.l.Unlock()
		return err
	}
}

// addBlock will write block into storage
func (w *Wallet) clearBlocks(tt int) error {
	db := w.getChainDB(tt)
	if db == nil {
		w.log.Error("Failed to add block, invalid token type")
		return fmt.Errorf("failed to get db")
	}
	iter := db.NewIterator(util.BytesPrefix([]byte(tcsType(tt))), nil)
	if !iter.First() {
		return nil
	}
	for {
		k := iter.Key()
		db.Delete(k, nil)
		if !iter.Next() {
			break
		}
	}
	return nil
}

// addBlock will write block into storage
func (w *Wallet) addBlocks(b *block.Block) error {
	opt := &opt.WriteOptions{
		Sync: true,
	}

	tokens := b.GetTransTokens()
	if tokens == nil {
		return fmt.Errorf("faile to get tokens from the block")
	}

	if len(tokens) == 1 {
		return w.addBlock(tokens[0], b)
	}
	db := w.getChainDB(b.GetTokenType(tokens[0]))
	if db == nil {
		w.log.Error("Failed to add block, invalid token type")
		return fmt.Errorf("failed to get db")
	}
	for _, token := range tokens {
		tt := b.GetTokenType(token)
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
	}
	bs, err := b.GetHash()
	if err != nil {
		return err
	}
	hs := ut.HexToStr(ut.CalculateHash(b.GetBlock(), "SHA3-256"))
	refkey := []byte(ReferenceType + "-" + hs + "-" + bs)
	_, err = w.getRawBlock(db, refkey)
	// if block already exist return error
	if err == nil {
		return fmt.Errorf("failed write the block, block already exist")
	}
	db.l.Lock()
	err = db.Put(refkey, b.GetBlock(), opt)
	db.l.Unlock()
	if err != nil {
		return err
	}
	for _, token := range tokens {
		bid, err := b.GetBlockID(token)
		tt := b.GetTokenType(token)
		if err != nil {
			return err
		}
		key := tcsKey(tt, token, bid)
		db.l.Lock()
		err = db.Put([]byte(key), refkey, opt)
		db.l.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// GetTokenBlock get token chain block from the storage
func (w *Wallet) GetTokenBlock(token string, tokenType int, blockID string) ([]byte, error) {
	return w.getBlock(tokenType, token, blockID)
}

// GetAllTokenBlocks get the tokecn chain blocks
func (w *Wallet) GetAllTokenBlocks(token string, tokenType int, blockID string) ([][]byte, string, error) {
	return w.getAllBlocks(tokenType, token, blockID)
}

// GetLatestTokenBlock get latest token block from the storage
func (w *Wallet) GetLatestTokenBlock(token string, tokenType int) *block.Block {
	return w.getLatestBlock(tokenType, token)
}

// GetLatestTokenBlock get latest token block from the storage
func (w *Wallet) GetGenesisTokenBlock(token string, tokenType int) *block.Block {
	return w.getGenesisBlock(tokenType, token)
}

// AddTokenBlock will write token block into storage
func (w *Wallet) AddTokenBlock(token string, b *block.Block) error {
	return w.addBlock(token, b)
}

// AddTokenBlock will write token block into storage
func (w *Wallet) CreateTokenBlock(b *block.Block) error {
	return w.addBlocks(b)
}

func (w *Wallet) ClearTokenBlocks(tokenType int) error {
	return w.clearBlocks(tokenType)
}
