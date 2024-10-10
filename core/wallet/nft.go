package wallet

import "fmt"

type NFT struct {
	TokenID     string  `gorm:"column:token_id;primaryKey" json:"token_id"`
	DID         string  `gorm:"column:did" json:"did"`
	TokenStatus int     `gorm:"column:token_status;" json:"token_status"`
	TokenValue  float64 `gorm:"column:token_value;" json:"token_value"`
}

// CreateNFT write NFT into db
func (w *Wallet) CreateNFT(nt *NFT) error {
	err := w.s.Write(NFTTokenStorage, nt)
	if err != nil {
		w.log.Error("Failed to write NFT into db", "err", err)
		return err
	}
	return nil
}

// GetAllNFT get all NFTs from db
func (w *Wallet) GetAllNFT(did string) []NFT {
	var tkns []NFT
	err := w.s.Read(NFTTokenStorage, &tkns, "did=?", did)
	if err != nil {
		return nil
	}
	return tkns
}

// GetNFT get NFT from db
func (w *Wallet) GetNFT(did string, nft string, lock bool) (*NFT, error) {
	var tkns NFT
	w.l.Lock()
	defer w.l.Unlock()
	if lock {
		err := w.s.Read(NFTTokenStorage, &tkns, "did=? AND token_id=? AND token_status <>?", did, nft, TokenIsLocked)
		if err != nil {
			return nil, err
		}
	} else {
		err := w.s.Read(NFTTokenStorage, &tkns, "did=? AND token_id=?", did, nft)
		if err != nil {
			return nil, err
		}
	}
	if tkns.TokenID != nft {
		return nil, fmt.Errorf("nft does not exist, failed to get nft")
	}
	if lock {
		tkns.TokenStatus = TokenIsLocked
		err := w.s.Update(NFTTokenStorage, &tkns, "did=? AND token_id=?", did, nft)
		if err != nil {
			return nil, err
		}
	}
	return &tkns, nil
}

func (w *Wallet) GetNFTToken(nft string) ([]NFT, error) {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var tokens []NFT
	w.log.Debug("nft=?", nft)
	err := w.s.Read(NFTTokenStorage, &tokens, "token_id=?", nft)
	if err != nil {
		w.log.Error("err", err)
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no smart contract token is available to commit")
	}

	return tokens, nil
}

func (w *Wallet) UpdateNFTStatus(nft string, tokenStatus int) error {
	w.dtl.Lock()
	defer w.dtl.Unlock()
	var nftToken NFT
	err := w.s.Read(NFTTokenStorage, &nft, "token_id=?", nftToken)
	if err != nil {
		w.log.Error("err", err)
		return err
	}
	nftToken.TokenStatus = tokenStatus
	err = w.s.Update(NFTTokenStorage, &nft, "token_id=?", nftToken)
	if err != nil {
		return err
	}
	return nil
}
