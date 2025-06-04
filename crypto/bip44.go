package crypto

import (
	"encoding/pem"
	"fmt"

	// "github.com/ethereum/go-ethereum/common"
	// "github.com/ethereum/go-ethereum/core/types"
	// "github.com/ethereum/go-ethereum/crypto"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

// HDWallet manages a hierarchical deterministic wallet
type HDWallet struct {
	Mnemonic  string
	Seed      []byte
	MasterKey *bip32.Key
	Accounts  map[uint32][]string // account index -> list of addresses
}

const (
	Purpose         uint32 = 44   // 44 for BIP-44
	RBTcoinType     uint32 = 1001 // for RBT tokens in Rubix mainnet
	TestRbtcoinType uint32 = 1002 // for test RBTs in Rubix testnet
)

const (
	NonWalletAccount uint32 = iota
	XellAcount
	SafePassAccount
)

// NewHDWallet creates a new HD wallet from a mnemonic or generates a new one
func NewHDWallet(mnemonic string) (*HDWallet, error) {
	if mnemonic == "" {
		// Generate a new 24-word mnemonic (256-bit entropy)
		mnemonic = BIPGenerateMnemonic()
	}

	// Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// Derive seed
	seed := bip39.NewSeed(mnemonic, "") // Empty passphrase

	// Derive master key
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %v", err)
	}

	return &HDWallet{
		Mnemonic:  mnemonic,
		Seed:      seed,
		MasterKey: masterKey,
		Accounts:  make(map[uint32][]string),
	}, nil
}

// DeriveKey derives a key for a given BIP-44 path (m/purpose'/coin_type'/account'/change/index)
func (w *HDWallet) DerivePrivateKey(path []uint32) (*bip32.Key, error) {
	// path := []uint32{
	// 	bip32.FirstHardenedChild + Purpose,     // e.g., 44'
	// 	bip32.FirstHardenedChild + coinType,    // e.g., 1001'
	// 	bip32.FirstHardenedChild + accountType, // e.g., 0'
	// 	change,                                 // 0 (external) or 1 (change)
	// 	index,                                  // 0, 1, 2, ...
	// }
	if len(path) != 5 {
		return nil, fmt.Errorf("failed to derive key, invalid HD path length: %v, should be precisely 5", len(path))
	}

	currentKey := w.MasterKey
	for _, i := range path {
		var err error
		currentKey, err = currentKey.NewChildKey(i)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key at index %d: %v", i, err)
		}
	}
	return currentKey, nil
}

func DeriveKeyPair(privateKey *bip32.Key, pwd string) ([]byte, []byte, error) {
	privKey := secp256k1.PrivKeyFromBytes(privateKey.Key)
	privkeybyte := privKey.Serialize()

	pubkeybyte := privKey.PubKey().SerializeUncompressed()
	var pemEncPriv []byte
	if pwd != "" {
		encBlock, err := Seal(pwd, privkeybyte)
		if err != nil {
			return nil, nil, err
		}
		_, err = UnSeal(pwd, encBlock)
		if err != nil {
			return nil, nil, err
		}
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: encBlock})
	} else {
		pemEncPriv = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privkeybyte})
	}
	pemEncPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubkeybyte})

	return pemEncPriv, pemEncPub, nil
}

// func GenerateHDChildKeys(coinType, accountType, change, index uint32, mnemonic, pwd string) ([]byte, []byte, error) {

// 	hdWallet, err := NewHDWallet(mnemonic)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	path := []uint32{
// 		bip32.FirstHardenedChild + Purpose,     // e.g., 44'
// 		bip32.FirstHardenedChild + coinType,    // e.g., 1001
// 		bip32.FirstHardenedChild + accountType, // e.g., 0'
// 		change,                                 // 0 (external) or 1 (change)
// 		index,                                  // 0, 1, 2, ...
// 	}
// 	privateKey, err := hdWallet.DerivePrivateKey(path)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	privKey, pubKey, err := DeriveKeyPair(privateKey, pwd)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	return privKey, pubKey, nil
// }
