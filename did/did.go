package did

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/nlss"
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
}

type DID struct {
	dir  string
	log  logger.Logger
	ipfs *ipfsnode.Shell
}

type DIDCrypto interface {
	GetDID() string
	Sign(hash string) ([]byte, []byte, error)
	Verify(hash string, didSig []byte, pvtSig []byte) (bool, error)
	PvtSign(hash []byte) ([]byte, error)
	PvtVerify(hash []byte, sign []byte) (bool, error)
}

func InitDID(dir string, log logger.Logger, ipfs *ipfsnode.Shell) *DID {
	did := &DID{
		dir:  dir,
		log:  log,
		ipfs: ipfs,
	}
	return did
}

func (d *DID) CreateDID(didCreate *DIDCreate) (string, error) {
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

	if didCreate.Type == BasicDIDMode || didCreate.Type == StandardDIDMode {
		f, err := os.Open(didCreate.ImgFile)
		if err != nil {
			d.log.Error("failed to open image", "err", err)
			return "", err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			d.log.Error("failed to decode image", "err", err)
			return "", err
		}
		bounds := img.Bounds()
		w, h := bounds.Max.X, bounds.Max.Y

		if w != 256 || h != 256 {
			d.log.Error("invalid image size", "err", err)
			return "", fmt.Errorf("invalid image")
		}
		pixels := make([]byte, 0)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				pixels = append(pixels, byte(r>>8))
				pixels = append(pixels, byte(g>>8))
				pixels = append(pixels, byte(b>>8))
			}
		}
		outPixels := make([]byte, 0)
		message := didCreate.Secret + util.GetMACAddress()
		dataHash := util.CalculateHash([]byte(message), "SHA3-256")
		offset := 0
		for y := 0; y < h; y++ {
			for x := 0; x < 24; x++ {
				for i := 0; i < 32; i++ {
					outPixels = append(outPixels, dataHash[i]^pixels[offset+i])
				}
				offset = offset + 32
				dataHash = util.CalculateHash(dataHash, "SHA3-256")
			}
		}

		err = util.CreatePNGImage(outPixels, w, h, dirName+"/public/"+DIDImgFileName)
		if err != nil {
			d.log.Error("failed to create image", "err", err)
			return "", err
		}
		pvtShare := make([]byte, 0)
		pubShare := make([]byte, 0)
		numBytes := len(outPixels)
		for i := 0; i < numBytes; i = i + 1024 {
			pvS, pbS := nlss.Gen2Shares(outPixels[i : i+1024])
			pvtShare = append(pvtShare, pvS...)
			pubShare = append(pubShare, pbS...)
		}
		err = util.CreatePNGImage(pvtShare, w*4, h*2, dirName+"/private/"+PvtShareFileName)
		if err != nil {
			d.log.Error("failed to create image", "err", err)
			return "", err
		}
		err = util.CreatePNGImage(pubShare, w*4, h*2, dirName+"/public/"+PubShareFileName)
		if err != nil {
			d.log.Error("failed to create image", "err", err)
			return "", err
		}
	}
	if didCreate.Type == WalletDIDMode {
		_, err := util.Filecopy(didCreate.DIDImgFileName, dirName+"/public/"+DIDImgFileName)
		if err != nil {
			d.log.Error("failed to copy did image", "err", err)
			return "", err
		}
		_, err = util.Filecopy(didCreate.PubImgFile, dirName+"/public/"+PubShareFileName)
		if err != nil {
			d.log.Error("failed to copy public share image", "err", err)
			return "", err
		}
	}
	if didCreate.Type == BasicDIDMode || didCreate.Type == ChildDIDMode {
		if didCreate.PrivPWD == "" {
			d.log.Error("password required for creating", "err", err)
			return "", err
		}
		pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: didCreate.PrivPWD})
		if err != nil {
			d.log.Error("failed to create keypair", "err", err)
			return "", err
		}
		err = util.FileWrite(dirName+"/private/"+PvtKeyFileName, pvtKey)
		if err != nil {
			return "", err
		}
		err = util.FileWrite(dirName+"/public/"+PubKeyFileName, pubKey)
		if err != nil {
			return "", err
		}
	} else {
		_, err := util.Filecopy(didCreate.PubKeyFile, dirName+"/public/"+PubKeyFileName)
		if err != nil {
			d.log.Error("failed to copy pub key", "err", err)
			return "", err
		}
	}

	if didCreate.Type == ChildDIDMode {
		if didCreate.MasterDID == "" {
			return "", fmt.Errorf("master did is missing")
		}
		err = util.FileWrite(dirName+"/public/"+MasterDIDFileName, []byte(didCreate.MasterDID))
		if err != nil {
			return "", err
		}
	} else {
		if didCreate.QuorumPWD == "" {
			if didCreate.PrivPWD != "" {
				didCreate.QuorumPWD = didCreate.PrivPWD
			} else {
				didCreate.QuorumPWD = DefaultPWD
			}
		}

		pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: didCreate.QuorumPWD})
		if err != nil {
			d.log.Error("failed to create keypair", "err", err)
			return "", err
		}
		err = util.FileWrite(dirName+"/private/"+QuorumPvtKeyFileName, pvtKey)
		if err != nil {
			return "", err
		}
		err = util.FileWrite(dirName+"/public/"+QuorumPubKeyFileName, pubKey)
		if err != nil {
			return "", err
		}
	}

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

func (d *DID) MigrateDID(didCreate *DIDCreate) (string, error) {
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

	_, err = util.Filecopy(didCreate.DIDImgFileName, dirName+"/public/"+DIDImgFileName)
	if err != nil {
		d.log.Error("failed to copy did image", "err", err)
		return "", err
	}
	_, err = util.Filecopy(didCreate.PubImgFile, dirName+"/public/"+PubShareFileName)
	if err != nil {
		d.log.Error("failed to copy public share", "err", err)
		return "", err
	}
	_, err = util.Filecopy(didCreate.PrivImgFile, dirName+"/private/"+PvtShareFileName)
	if err != nil {
		d.log.Error("failed to copy private share key", "err", err)
		return "", err
	}
	if didCreate.Type == BasicDIDMode {
		if didCreate.PrivKeyFile == "" || didCreate.PubKeyFile == "" {
			if didCreate.PrivPWD == "" {
				d.log.Error("password required for creating", "err", err)
				return "", err
			}
			pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: didCreate.PrivPWD})
			if err != nil {
				d.log.Error("failed to create keypair", "err", err)
				return "", err
			}
			err = util.FileWrite(dirName+"/private/"+PvtKeyFileName, pvtKey)
			if err != nil {
				return "", err
			}
			err = util.FileWrite(dirName+"/public/"+PubKeyFileName, pubKey)
			if err != nil {
				return "", err
			}
		} else {
			_, err = util.Filecopy(didCreate.PrivKeyFile, dirName+"/private/"+PvtKeyFileName)
			if err != nil {
				d.log.Error("failed to copy private key", "err", err)
				return "", err
			}
			_, err = util.Filecopy(didCreate.PubKeyFile, dirName+"/public/"+PubKeyFileName)
			if err != nil {
				d.log.Error("failed to copy pub key", "err", err)
				return "", err
			}
		}
	} else {
		_, err := util.Filecopy(didCreate.PubKeyFile, dirName+"/public/"+PubKeyFileName)
		if err != nil {
			d.log.Error("failed to copy pub key", "err", err)
			return "", err
		}
	}

	if didCreate.QuorumPWD == "" {
		if didCreate.PrivPWD != "" {
			didCreate.QuorumPWD = didCreate.PrivPWD
		} else {
			didCreate.QuorumPWD = DefaultPWD
		}
	}
	if didCreate.QuorumPrivKeyFile == "" || didCreate.QuorumPubKeyFile == "" {
		pvtKey, pubKey, err := crypto.GenerateKeyPair(&crypto.CryptoConfig{Alg: crypto.ECDSAP256, Pwd: didCreate.QuorumPWD})
		if err != nil {
			d.log.Error("failed to create keypair", "err", err)
			return "", err
		}
		err = util.FileWrite(dirName+"/private/"+QuorumPvtKeyFileName, pvtKey)
		if err != nil {
			return "", err
		}
		err = util.FileWrite(dirName+"/public/"+QuorumPubKeyFileName, pubKey)
		if err != nil {
			return "", err
		}
	} else {
		_, err = util.Filecopy(didCreate.QuorumPrivKeyFile, dirName+"/private/"+QuorumPvtKeyFileName)
		if err != nil {
			return "", err
		}
		_, err = util.Filecopy(didCreate.QuorumPubKeyFile, dirName+"/public/"+QuorumPubKeyFileName)
		if err != nil {
			return "", err
		}
	}

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
	fmt.Printf("DID : %s, Time to create DID & Keys : %v", did, dif)
	return did, nil
}

type object struct {
	Hash string
}

func (d *DID) getDirHash(dir string) (string, error) {
	stat, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}

	sf, err := files.NewSerialFile(dir, false, stat)
	if err != nil {
		return "", err
	}
	defer sf.Close()
	slf := files.NewSliceDirectory([]files.DirEntry{files.FileEntry(filepath.Base(dir), sf)})
	defer slf.Close()
	reader := files.NewMultiFileReader(slf, true)

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

	if resp.Error != nil {
		return "", resp.Error
	}
	defer resp.Output.Close()
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

	if final == "" {
		return "", errors.New("no results received")
	}

	return final, nil
}
