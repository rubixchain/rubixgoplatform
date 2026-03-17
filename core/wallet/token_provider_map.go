package wallet

import (
	"io"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types"
)

func (w *Wallet) Add(r io.Reader, did string, role int, skipProvider ...bool) (string, error) {
	var provCtx *types.IPFSProviderContext
	if len(skipProvider) == 0 || !skipProvider[0] {
		provCtx = &types.IPFSProviderContext{
			DID:          did,
			Role:         role,
			ResourceType: constants.IPFSResourceTokenState,
		}
	}
	result, err := w.ipfsOps.Add(r, provCtx)
	if err != nil {
		w.log.Error("Error adding file to ipfs", "error", err)
		return "", err
	}
	return result, nil
}

func (w *Wallet) Pin(hash string, role int, did string, transactionId string, sender string, receiver string, tokenValue float64, skipProviderDetails ...bool) (bool, error) {
	var provCtx *types.IPFSProviderContext
	if len(skipProviderDetails) == 0 || !skipProviderDetails[0] {
		provCtx = &types.IPFSProviderContext{
			DID:           did,
			Role:          role,
			TransactionID: transactionId,
			ResourceType:  constants.IPFSResourceTokenState,
			ResourceID:    hash,
			Initiator:     sender,
			Owner:         receiver,
			TokenValue:    tokenValue,
		}
	}
	err := w.ipfsOps.Pin(hash, provCtx)
	if err != nil {
		w.log.Error("Failed to pin token", "hash", hash, "error", err)
		return false, err
	}
	return true, nil
}

func (w *Wallet) Unpin(hash string, role int, did string) (bool, error) {
	provCtx := &types.IPFSProviderContext{
		DID:          did,
		Role:         role,
		ResourceType: constants.IPFSResourceTokenState,
		ResourceID:   hash,
	}
	err := w.ipfsOps.Unpin(hash, provCtx)
	if err != nil {
		w.log.Error("Failed to unpin token", "hash", hash, "error", err)
		return false, err
	}
	return true, nil
}

// AddWithProviderMap adds data to IPFS with provider context and returns both the hash and context
// for callers that need to batch-record provider details later.
func (w *Wallet) AddWithProviderMap(r io.Reader, did string, role int) (string, types.IPFSProviderContext, error) {
	provCtx := types.IPFSProviderContext{
		DID:          did,
		Role:         role,
		ResourceType: constants.IPFSResourceTokenState,
	}
	hash, err := w.ipfsOps.Add(r, &provCtx)
	if err != nil {
		return "", provCtx, err
	}
	return hash, provCtx, nil
}
