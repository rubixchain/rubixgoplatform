package core

import (
	"bytes"
	"fmt"

	"github.com/google/uuid"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// expandChildMintEntries rewrites child-mint entries into parent-execute +
// child-deploy pairs. Child ID is caller-supplied (n.NFTId) when present,
// else IPFS-add of (parentNFTId + uuid). Child value inherits parent's when
// the request omits it. childToParent feeds post-consensus persistence so
// the originator's tokens.parent_token_id gets set.
func (c *Core) expandChildMintEntries(request *models.TransactionRequest) ([]model.MintedChild, map[string]string, error) {
	// Non-nil slice/map so JSON marshals as "[]" / {} when no children minted.
	empty := []model.MintedChild{}
	emptyMap := map[string]string{}

	if request == nil || len(request.Tokens.NFT) == 0 {
		return empty, emptyMap, nil
	}

	// Nothing to do if no entry asks for a child mint.
	hasChildMint := false
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId != "" {
			hasChildMint = true
			break
		}
	}
	if !hasChildMint {
		return empty, emptyMap, nil
	}

	if request.Tokens.TransferNFTOwnership {
		return nil, nil, fmt.Errorf("child-mint cannot be combined with transferNftOwnership=true")
	}

	// CID-validate parentNFTId so typos fail here, not in the wallet lookup.
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" {
			continue
		}
		if err := util.ValidateCIDFormat(n.ParentNFTId); err != nil {
			return nil, nil, fmt.Errorf("parentNFTId %w", err)
		}
	}

	// Caller-supplied child IDs: CID-shaped, unique in the request, and not
	// already deployed locally.
	suppliedChildIDs := make(map[string]struct{})
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" || n.NFTId == "" {
			continue
		}
		if err := util.ValidateCIDFormat(n.NFTId); err != nil {
			return nil, nil, fmt.Errorf("child nftId %w", err)
		}
		if _, dup := suppliedChildIDs[n.NFTId]; dup {
			return nil, nil, fmt.Errorf("child nftId %s appears more than once in the same request", n.NFTId)
		}
		suppliedChildIDs[n.NFTId] = struct{}{}
		exists, err := c.w.TokenExists(c.Ctx, nil, n.NFTId)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to check existence of child nftId %s: %w", n.NFTId, err)
		}
		if exists {
			return nil, nil, fmt.Errorf("child nftId %s already exists locally; remove parentNFTId to execute it or supply a new id", n.NFTId)
		}
	}

	// Reject the same NFT being both an explicit execute target and a child-mint parent.
	explicitParents := make(map[string]struct{})
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" && n.NFTId != "" {
			explicitParents[n.NFTId] = struct{}{}
		}
	}

	// Confirm each parent exists locally and remember its value for inheritance.
	parentValues := make(map[string]float64)
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" {
			continue
		}
		if _, seen := parentValues[n.ParentNFTId]; seen {
			continue
		}
		if _, clash := explicitParents[n.ParentNFTId]; clash {
			return nil, nil, fmt.Errorf("parent NFT %s appears as both an explicit execute target and a child-mint parent in the same request", n.ParentNFTId)
		}
		tok, err := c.w.GetTokenByTokenID(n.ParentNFTId)
		if err != nil {
			return nil, nil, fmt.Errorf("parent NFT %s not found in wallet: %w", n.ParentNFTId, err)
		}
		parentValues[n.ParentNFTId] = tok.TokenValue
	}

	// Preserve first-seen order so execute entries are emitted predictably.
	parentOrder := make([]string, 0, len(parentValues))
	seenParent := make(map[string]struct{})
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" {
			continue
		}
		if _, seen := seenParent[n.ParentNFTId]; seen {
			continue
		}
		seenParent[n.ParentNFTId] = struct{}{}
		parentOrder = append(parentOrder, n.ParentNFTId)
	}

	expanded := make([]models.NFTInfo, 0, len(request.Tokens.NFT)+len(parentOrder))

	// One execute entry per distinct parent; downstream fills in the value.
	for _, p := range parentOrder {
		expanded = append(expanded, models.NFTInfo{NFTId: p})
	}

	// Emit a deploy entry per child-mint entry; pass others through unchanged.
	mintedChildren := make([]model.MintedChild, 0)
	childToParent := make(map[string]string)
	for _, n := range request.Tokens.NFT {
		if n.ParentNFTId == "" {
			expanded = append(expanded, n)
			continue
		}

		var childID string
		if n.NFTId != "" {
			childID = n.NFTId
		} else {
			childUUID := uuid.New().String()
			payload := []byte(n.ParentNFTId + childUUID)
			generated, err := IpfsAddWithBackoff(c.ipfs, bytes.NewReader(payload))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to IPFS-add child NFT payload for parent %s: %w", n.ParentNFTId, err)
			}
			childID = generated
		}

		// Inherit parent's value when the request omits one.
		childValue := n.Value
		if childValue == 0 {
			childValue = parentValues[n.ParentNFTId]
		}

		// Synthesized entry: no ParentNFTId here so we don't re-expand it;
		// lineage is carried in childToParent for persistence.
		expanded = append(expanded, models.NFTInfo{
			NFTId: childID,
			Value: childValue,
			Data:  n.Data,
		})

		mintedChildren = append(mintedChildren, model.MintedChild{
			ParentNFTId: n.ParentNFTId,
			ChildNFTId:  childID,
		})
		childToParent[childID] = n.ParentNFTId
	}

	request.Tokens.NFT = expanded
	return mintedChildren, childToParent, nil
}
