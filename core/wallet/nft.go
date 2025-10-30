package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type NFT struct {
	TokenID     string  `gorm:"column:token_id;primaryKey" json:"token_id"`
	DID         string  `gorm:"column:did" json:"did"`
	TokenStatus int     `gorm:"column:token_status;" json:"token_status"`
	TokenValue  float64 `gorm:"column:token_value;" json:"token_value"`
	Metadata    string  `gorm:"column:metadata;" json:"metadata"`
	Filename    string  `gorm:"column:filename;" json:"filename"`
}

type SyncedNFT struct {
	TokenID       string  `gorm:"column:token_id;primaryKey" json:"token_id"`
	TokenValue    float64 `gorm:"column:token_value;" json:"token_value"`
	OwnerDID      string  `gorm:"column:owner_did"`
	PublisherDID  string  `gorm:"column:publisher_did"`
	TransactionID string  `gorm:"column:transaction_id"`
	BlockHash     string  `gorm:"column:block_hash"`
	BlockHeight   uint64  `gorm:"column:block_height"`
	SyncStatus    int     `gorm:"column:sync_status"`
}

type NFTContent struct {
	NFTId            string `json:"nft_id"`
	DeployerDID      string `json:"deployer_did"`
	ArtifactFileName string `json:"artifact_filename"`
	Artifact         []byte `json:"artifact"`
	MetadataFileName string `json:"metadata_filename"`
	Metadata         []byte `json:"metadata"`
}

// CreateNFT write NFT into db
func (w *Wallet) CreateNFT(nt *NFT, local bool) error {
	// TODO: Update should only occur in UpdateNFT status function
	var err error
	if local {
		err = w.s.Update(NFTTokenStorage, nt, "token_id=?", nt.TokenID)
		if err != nil {
			w.log.Error("Failed to update NFT into db", "err", err)
			return err
		}
	} else {
		err := w.s.Write(NFTTokenStorage, nt)
		if err != nil {
			w.log.Error("Failed to write NFT into db", "err", err)
			return err
		}
	}
	return nil
}

// GetAllNFT get all NFTs from db
func (w *Wallet) GetAllNFT() ([]NFT, error) {
	var tkns []NFT
	err := w.s.Read(NFTTokenStorage, &tkns, "token_id != ?", "")
	if err != nil {
		return nil, err
	}
	return tkns, nil
}

// GetNFTsByDid get all the NFTs of that did from db
func (w *Wallet) GetNFTsByDid(did string) ([]NFT, error) {
	var tkns []NFT
	err := w.s.Read(NFTTokenStorage, &tkns, "did=?", did)
	if err != nil {
		return nil, err
	}
	return tkns, nil
}

func (w *Wallet) GetNFTToken(nftID string) (*NFT, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var tokens *NFT

	err := w.s.Read(NFTTokenStorage, &tokens, "token_id=?", nftID)
	if err != nil {
		return nil, fmt.Errorf("unable to find NFT Token %v, err: %v", nftID, err)
	}

	return tokens, nil
}

func (w *Wallet) IsNFTExists(nftID string) bool {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var tokens *NFT

	err := w.s.Read(NFTTokenStorage, &tokens, "token_id=?", nftID)
	if err != nil {
		return false
	} else {
		return true
	}
}

func (w *Wallet) UpdateNFTStatus(nft string, tokenStatus int, local bool, receiverDid string, saleAmount float64) error {
	// Empty receiver DID indicates self execution of NFT and hence
	// any change in NFTToken table must be skipped
	if receiverDid != "" {
		w.dtl.Lock()
		defer w.dtl.Unlock()
		var nftToken NFT
		err := w.s.Read(NFTTokenStorage, &nftToken, "token_id=?", nft)
		if err != nil {
			w.log.Error("err", err)
			return err
		}

		nftToken.TokenValue = floatPrecision(saleAmount, 3)
		nftToken.DID = receiverDid
		if local {
			nftToken.TokenStatus = TokenIsFree
		} else {
			nftToken.TokenStatus = tokenStatus
		}

		err = w.s.Update(NFTTokenStorage, &nftToken, "token_id=?", nft)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) StoreNFTFilesToPSQL(nftID, deplaoyerDID, ArtifactHash, outputDir string) error {
	start := time.Now()
	w.log.Info("Starting to store NFT files from directory", "path", outputDir)

	// Intialize nft content
	nftContent := &NFTContent{
		NFTId:       nftID,
		DeployerDID: deplaoyerDID,
	}

	fileNames := make([]string, 0)
	artifactsBytes := make([][]byte, 0)
	// Walk recursively through the folder
	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			w.log.Error("Error accessing path", "path", path, "err", err)
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read file as bytes
		fileBytes, err := os.ReadFile(path)
		if err != nil {
			w.log.Error("Failed to read file", "file", path, "err", err)
			return err
		}

		// Extract relative filename (e.g., metadata.json, image.png)
		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			relPath = filepath.Base(path)
		}

		fileNames = append(fileNames, relPath)
		artifactsBytes = append(artifactsBytes, fileBytes)

		return nil
	})

	if err != nil {
		w.log.Error("Failed to walk NFT directory", "err", err)
		return err
	}

	if len(fileNames) < 2 || len(artifactsBytes) < 2 {
		return fmt.Errorf("expected at least two files (metadata + artifact), got %d, received file name : %v", len(fileNames), fileNames)
	}

	// Insert into PostgreSQL
	nftContent.NFTId = nftID
	nftContent.DeployerDID = deplaoyerDID
	nftContent.MetadataFileName = fileNames[0]
	nftContent.Metadata = artifactsBytes[0]
	nftContent.ArtifactFileName = fileNames[1]
	nftContent.Artifact = artifactsBytes[1]
	err = w.AddNFTContentToPSQl(nftContent)
	if err != nil {
		w.log.Error("Failed to insert NFT file into DB, ", "files", fileNames, "err", err)
		return err
	}

	w.log.Info("Stored NFT files, ", "filenames", fileNames, "size", len(artifactsBytes[0]), len(artifactsBytes[1]))

	w.log.Info("Successfully stored all NFT files", "duration", time.Since(start))
	return nil
}
