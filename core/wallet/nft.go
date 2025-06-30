package wallet

import "fmt"

type NFT struct {
	TokenID     string  `gorm:"column:token_id;primaryKey" json:"token_id"`
	DID         string  `gorm:"column:did" json:"did"`
	TokenStatus int     `gorm:"column:token_status;" json:"token_status"`
	TokenValue  float64 `gorm:"column:token_value;" json:"token_value"`
	Metadata    string  `gorm:"column:metadata;" json:"metadata"`
	Filename    string  `gorm:"column:filename;" json:"filename"`
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
