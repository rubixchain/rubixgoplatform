package did

import (
	"fmt"
	"image"
	"os"

	"github.com/EnsurityTechnologies/logger"
	"github.com/EnsurityTechnologies/uuid"
	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/core/nlss"
	"github.com/rubixchain/rubixgoplatform/core/util"
)

type DID struct {
	cfg  *config.Config
	log  logger.Logger
	ipfs *ipfsnode.Shell
}

func InitDID(cfg *config.Config, log logger.Logger, ipfs *ipfsnode.Shell) *DID {
	did := &DID{
		cfg:  cfg,
		log:  log,
		ipfs: ipfs,
	}
	return did
}

func (d *DID) CreateDID(message string, imgFile string) (string, error) {
	f, err := os.Open(imgFile)
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
	message = message + util.GetMACAddress()
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
	temp := uuid.New()
	dirName := d.cfg.DirPath + "Rubix/" + temp.String()
	err = os.MkdirAll(dirName+"/public", os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}

	err = os.MkdirAll(dirName+"/private", os.ModeDir|os.ModePerm)
	if err != nil {
		d.log.Error("failed to create directory", "err", err)
		return "", err
	}
	err = util.CreatePNGImage(outPixels, w, h, dirName+"/public/DID.png")
	if err != nil {
		d.log.Error("failed to create image", "err", err)
		return "", err
	}
	// f, err = os.Open(dirName + "/DID.png")
	// if err != nil {
	// 	d.log.Error("failed to open image", "err", err)
	// 	return "", err
	// }

	// hash, err := d.ipfs.AddNoPin(f)
	// if err != nil {
	// 	f.Close()
	// 	d.log.Error("failed to get hash from ipfs", "err", err)
	// 	return "", err
	// }
	// f.Close()
	// didPath := d.cfg.DirPath + "Rubix/" + hash
	// err = os.Rename(dirName, didPath)
	// if err != nil {
	// 	d.log.Error("failed to rename directory", "err", err)
	// 	return "", err
	// }
	pvtShare := make([]byte, 0)
	pubShare := make([]byte, 0)
	numBytes := len(outPixels)
	for i := 0; i < numBytes; i = i + 1024 {
		pvS, pbS := nlss.Gen2Shares(outPixels[i : i+1024])
		pvtShare = append(pvtShare, pvS...)
		pubShare = append(pubShare, pbS...)
	}
	err = util.CreatePNGImage(pvtShare, w*4, h*2, dirName+"/private/pvtShare.png")
	if err != nil {
		d.log.Error("failed to create image", "err", err)
		return "", err
	}
	err = util.CreatePNGImage(pubShare, w*4, h*2, dirName+"/public/pubShare.png")
	if err != nil {
		d.log.Error("failed to create image", "err", err)
		return "", err
	}
	// f, err = os.Open(didPath)
	// if err != nil {
	// 	d.log.Error("failed to open directory", "err", err)
	// 	return "", err
	// }
	dirHash, err := d.ipfs.AddDir(dirName + "/public")
	if err != nil {
		d.log.Error("failed to get hash of the directory", "err", err)
		return "", err
	}
	fmt.Printf("DIrectory Hash : %s\n", dirHash)
	didPath := d.cfg.DirPath + "Rubix/" + dirHash
	err = os.Rename(dirName, didPath)
	if err != nil {
		d.log.Error("failed to rename directory", "err", err)
		return "", err
	}
	return dirHash, nil
}
