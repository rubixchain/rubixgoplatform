package did

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/types"
	"github.com/rubixchain/rubixgoplatform/util"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

type DIDChan struct {
	ID      string
	InChan  chan interface{}
	OutChan chan interface{}
	Finish  chan bool
	Req     *ensweb.Request
	Timeout time.Duration

	// Password caching fields for request-scoped authentication
	CachedPassword string       // Cached password for this request
	PasswordSet    bool         // Flag to track if password is set
	PasswordMutex  sync.RWMutex // Thread-safe access to password cache
}

type DID struct {
	dir  string
	log  logger.Logger
	ipfs *ipfsnode.Shell
}

func InitDID(dir string, log logger.Logger, ipfs *ipfsnode.Shell) *DID {
	did := &DID{
		dir:  dir,
		log:  log,
		ipfs: ipfs,
	}
	return did
}

func (d *DID) CreateDID(didCreate *types.DIDCreate) (string, error) {
	t1 := time.Now()
	temp := uuid.New()
	dirName := d.dir + temp.String()
	err := os.MkdirAll(dirName+"/public", os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	err = os.MkdirAll(dirName+"/private", os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	//In lite mode, did is simply the SHA-256 hash  of the public key
	if didCreate.PrivPWD == "" {
		d.log.Error("password required for creating", "err", err)
		return "", err
	}

	var mnemonic string
	if didCreate.Mnemonic == "" { // create new mnemonic phrase
		d.log.Debug("No mnemonic provided , creating new keypair")
		mnemonic = crypto.BIPGenerateMnemonic()
	} else { // use the input mnemonic to re-generate the key pair
		mnemonic = didCreate.Mnemonic
	}

	masterKey, err := crypto.BIPGenerateMasterKeyFromMnemonic(mnemonic)
	if err != nil {
		d.log.Error("failed to create keypair", "err", err)
	}

	//generating private and public key pair
	pvtKey, pubKey, err := crypto.BIPGenerateChild(string(masterKey), didCreate.ChildPath, didCreate.PrivPWD)
	if err != nil {
		d.log.Error("failed to create child", "err", err)
	}

	// write mnemonic into a file
	err = util.FileWrite(dirName+"/private/"+constants.MnemonicFileName, []byte(mnemonic))
	if err != nil {
		d.log.Error("failed to write mnemonic file", "err", err)
		return "", err
	}

	// write private key into a file
	err = util.FileWrite(dirName+"/private/"+constants.PvtKeyFileName, pvtKey)
	if err != nil {
		return "", err
	}

	// write public key into a file
	err = util.FileWrite(dirName+"/public/"+constants.PubKeyFileName, pubKey)
	if err != nil {
		return "", err
	}

	// Test the generated key pair by reading it from the file,
	// signing on a message using the private key, and verify the signature using the public key
	privKeyTest, err := ioutil.ReadFile(dirName + "/private/" + constants.PvtKeyFileName)
	if err != nil {
		return "", err
	}
	Privkey, _, err := crypto.DecodeBIPKeyPair(didCreate.PrivPWD, privKeyTest, nil)
	if err != nil {
		return "", err
	}
	privkeyback := secp256k1.PrivKeyFromBytes(Privkey)
	privKeySer := privkeyback.ToECDSA()

	// sign on the message "test" using the private key
	pvtKeySign, err := crypto.BIPSign(privKeySer, []byte("test"))
	if err != nil {
		return "", err
	}

	pubKeyTest, err := ioutil.ReadFile(dirName + "/public/" + constants.PubKeyFileName)
	if err != nil {
		return "", err
	}
	_, pubKeyByte, err := crypto.DecodeBIPKeyPair("", nil, pubKeyTest)
	if err != nil {
		return "", err
	}
	pubkeyback, err := secp256k1.ParsePubKey(pubKeyByte)
	if err != nil {
		return "", fmt.Errorf("failed to parse public key at creation of DID: %w, use BIP DID", err)
	}
	if pubkeyback == nil {
		return "", fmt.Errorf("public key parsing returned nil at creation of DID")
	}
	pubKeySer := pubkeyback.ToECDSA()

	// verify the signature using the public key
	if !crypto.BIPVerify(pubKeySer, []byte("test"), pvtKeySign) {
		return "", fmt.Errorf("failed to verify private key singature")
	}

	//passing the diroctory of public key file to add it to ipfs and exctract the hash
	did, err := d.getDirHash(dirName + "/public/")
	if err != nil {
		return "", err
	}

	newDIrName := d.dir + did

	err = os.MkdirAll(newDIrName, os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	err = util.DirCopy(dirName+"/public", newDIrName)
	if err != nil {
		d.log.Error("failed to copy directory", "err", err)
		return "", err
	}

	err = util.DirCopy(dirName+"/private", newDIrName)
	if err != nil {
		d.log.Error("failed to copy directory", "err", err)
		return "", err
	}
	os.RemoveAll(dirName)
	t2 := time.Now()
	dif := t2.Sub(t1)
	d.log.Info(fmt.Sprintf("DID : %s, Time to create DID & Keys : %v", did, dif))
	return did, nil
}

type object struct {
	Hash string
}

// Calculate the hash of a directory using IPFS
func (d *DID) getDirHash(dir string) (string, error) {
	// Get information about the directory
	stat, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}

	// Create a new SerialFile using the directory information
	sf, err := files.NewSerialFile(dir, false, stat)
	if err != nil {
		return "", err
	}
	defer sf.Close()

	// Create a new SliceDirectory with the SerialFile
	slf := files.NewSliceDirectory([]files.DirEntry{files.FileEntry(filepath.Base(dir), sf)})
	defer slf.Close()

	// Create a MultiFileReader with the SliceDirectory
	reader := files.NewMultiFileReader(slf, true)

	// Send a request to IPFS to add the directory
	resp, err := d.ipfs.Request("add").
		Option("recursive", true).
		Option("cid-version", 1).
		Option("hash", "sha3-256").
		Body(reader).
		Send(context.Background())
	if err != nil {
		return "", err
	}

	defer resp.Close()

	// Check for errors in the response
	if resp.Error != nil {
		return "", resp.Error
	}
	defer resp.Output.Close()

	// Decode the JSON response and extract the hash
	dec := json.NewDecoder(resp.Output)
	var final string
	for {
		var out object
		err = dec.Decode(&out)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		final = out.Hash
	}

	// Check if the final hash is empty
	if final == "" {
		return "", errors.New("no results received")
	}

	return final, nil
}

// CreateDIDFromPubKey creates a DID from the provided public key for BIP wallet
func (d *DID) CreateDIDFromPubKey(didCreate *types.DIDCreate) (string, error) {
	t1 := time.Now()
	temp := uuid.New()
	dirName := d.dir + temp.String()

	//create a temporary directory
	err := os.MkdirAll(dirName+"/public", os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	// Convert hex string back to bytes
	pubKeyBytes, err := hex.DecodeString(didCreate.PubKey)
	if err != nil {
		d.log.Error("Failed to decode hex string, err", err)
	}

	// It is important to save the pem encrypted public key, so that the quorums can use
	// the existing sign-verification function, which includes pem decoding of public key
	pemEncPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})
	//write public key into the temporary directory
	err = util.FileWrite(dirName+"/public/"+constants.PubKeyFileName, pemEncPub)
	if err != nil {
		return "", err
	}

	pubKeyTest, err := os.ReadFile(dirName + "/public/" + constants.PubKeyFileName)
	if err != nil {
		return "", err
	}

	_, pubKeyByte, err := crypto.DecodeBIPKeyPair("", nil, pubKeyTest)
	if err != nil {
		d.log.Error("failed to decode pub key bytes")
		return "", err
	}
	_, err = secp256k1.ParsePubKey(pubKeyByte)
	if err != nil {
		d.log.Error("failed to parse public key, err:", err)
		return "", err
	}

	//passing the temp diroctory of public key file to add it to ipfs and exctract the hash
	did, err := d.getDirHash(dirName + "/public/")
	if err != nil {
		return "", err
	}

	//create new directory with the name including newly created did,
	newDIrName := d.dir + did
	err = os.MkdirAll(newDIrName, os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	// and store the public key in the new directory
	err = util.DirCopy(dirName+"/public", newDIrName)
	if err != nil {
		d.log.Error("failed to copy directory", "err", err)
		return "", err
	}
	//delete the temporary directory
	os.RemoveAll(dirName)
	t2 := time.Now()
	dif := t2.Sub(t1)
	d.log.Info(fmt.Sprintf("DID : %s, Time to create DID & Keys : %v", did, dif))
	return did, nil
}
