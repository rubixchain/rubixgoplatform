package core

import (
	"github.com/rubixchain/rubixgoplatform/core/wallet"
)

func (c *Core) GetAllRBTs() ([]wallet.SyncedRBT, error) {
	RBTs, err := c.w.GetAllRBTs()
	if err != nil {
		return nil, err
	}
	return RBTs, nil
}

func (c *Core) GetAllFTs() ([]wallet.SyncedFT, error) {
	FTs, err := c.w.GetAllFTs()
	if err != nil {
		return nil, err
	}
	return FTs, nil
}

func (c *Core) GetAllNFTs() ([]wallet.SyncedNFT, error) {
	NFTs, err := c.w.GetAllNFTs()
	if err != nil {
		return nil, err
	}
	return NFTs, nil
}

func (c *Core) GetAllSmartContracts() ([]wallet.SyncedSmartContract, error) {
	SCs, err := c.w.GetAllSmartContracts()
	if err != nil {
		return nil, err
	}
	return SCs, nil
}

func (c *Core) GetRBTsbyDID(DID string) ([]wallet.SyncedRBT, error) {
	RBTs, err := c.w.GetAllRBTbyDID(DID)
	if err != nil {
		return nil, err
	}
	return RBTs, nil
}

func (c *Core) GetFTsbyDID(DID string) ([]wallet.FTToken, error) {
	FTs, err := c.w.GetAllFTsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return FTs, nil
}

// returning all the NFTs(syncedNFT) by DID
func (c *Core) GetNFTsbyDID(DID string) ([]wallet.SyncedNFT, error) {
	NFTs, err := c.w.GetAllNFTsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return NFTs, nil
}

// returning all the SmartContracts(syncedSmartContract) by DID
func (c *Core) GetSmartContractsbyDID(DID string) ([]wallet.SyncedSmartContract, error) {
	SCs, err := c.w.GetAllSmartContractsbyDID(DID)
	if err != nil {
		return nil, err
	}
	return SCs, nil
}

