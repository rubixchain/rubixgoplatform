package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	"github.com/rubixchain/rubixgoplatform/types/models"
	"github.com/rubixchain/rubixgoplatform/util"
)

// propertiesResolveBudget bounds the WHOLE resolution, not each hop.
// ValidateTransaction takes no context.Context, so this is the only ceiling
// short of the listener's 10-minute WriteTimeout.
const propertiesResolveBudget = 30 * time.Second

// GetPropertiesTokenID derives the properties token ID governing nftTokenID.
// It hashes "<nftID>-properties" rather than using the literal string, which
// util.GetRbtIDElements would misread as an NFT or smart contract.
func (c *Core) GetPropertiesTokenID(nftTokenID string) (string, error) {
	if nftTokenID == "" {
		return "", fmt.Errorf("GetPropertiesTokenID: nft token ID is empty")
	}
	seed := nftTokenID + "-properties"
	id, err := c.ipfsOps.Add(bytes.NewBufferString(seed), nil)
	if err != nil {
		return "", fmt.Errorf("GetPropertiesTokenID: failed to derive properties token ID for %s: %w", nftTokenID, err)
	}
	return id, nil
}

// ResolveNFTProperties resolves the properties governing nftTokenID, returning
// (nil, nil) when the NFT has none so legacy NFTs stay unrestricted. Callers
// MUST treat any error as a rejection: this gate fails closed.
func (c *Core) ResolveNFTProperties(nftTokenID string) (*models.ResolvedProperties, error) {
	type outcome struct {
		res *models.ResolvedProperties
		err error
	}
	ch := make(chan outcome, 1)

	go func() {
		res, err := c.resolveNFTPropertiesInner(nftTokenID)
		ch <- outcome{res: res, err: err}
	}()

	select {
	case o := <-ch:
		return o.res, o.err
	case <-time.After(propertiesResolveBudget):
		return nil, fmt.Errorf("ResolveNFTProperties: timed out after %s resolving properties for NFT %s",
			propertiesResolveBudget, nftTokenID)
	}
}

func (c *Core) resolveNFTPropertiesInner(nftTokenID string) (*models.ResolvedProperties, error) {
	propsTokenID, err := c.GetPropertiesTokenID(nftTokenID)
	if err != nil {
		return nil, err
	}

	tx, role, err := c.w.GetLatestTransactionAndRoleByTokenID(propsTokenID)
	if err != nil {
		return nil, fmt.Errorf("ResolveNFTProperties: reading local chain tip for %s: %w", propsTokenID, err)
	}

	if tx == nil {
		// Not held locally. Sync from the NFT's peer, then re-read.
		synced, syncErr := c.syncPropertiesChain(nftTokenID, propsTokenID)
		if !synced {
			// No peer could be reached, so "no properties" and "could not
			// check" are indistinguishable; guessing either way fails open.
			return nil, fmt.Errorf("ResolveNFTProperties: cannot determine whether NFT %s has properties: %w",
				nftTokenID, syncErr)
		}

		// Re-read locally: SyncTransactionChainsFromPeer returns nil even when
		// the apply failed, so only the local tip proves anything. Without this
		// a failed sync reads as "no properties" and the gate fails open.
		tx, role, err = c.w.GetLatestTransactionAndRoleByTokenID(propsTokenID)
		if err != nil {
			return nil, fmt.Errorf("ResolveNFTProperties: re-reading chain tip for %s after sync: %w", propsTokenID, err)
		}
		if tx == nil {
			// The peer answered and has no properties chain for this NFT —
			// the legitimate unrestricted case.
			c.log.Debug("ResolveNFTProperties: no properties token after sync; treating NFT as unrestricted",
				"nft", nftTokenID, "propertiesToken", propsTokenID)
			return nil, nil
		}
	}

	if role == int16(models.GetTokenRoleID(constants.TokenRole_Burn)) {
		return nil, fmt.Errorf("ResolveNFTProperties: properties token %s for NFT %s is burnt", propsTokenID, nftTokenID)
	}

	docCID, deployer, err := c.propertiesDocCIDFromChain(propsTokenID, tx)
	if err != nil {
		return nil, err
	}

	doc, err := c.fetchPropertiesDocument(docCID)
	if err != nil {
		return nil, fmt.Errorf("ResolveNFTProperties: NFT %s: %w", nftTokenID, err)
	}

	resolved := &models.ResolvedProperties{
		PropertiesTokenID: propsTokenID,
		DocCID:            docCID,
		Doc:               doc,
		Deployer:          deployer,
	}

	// Second hop. A present but unfetchable CID must reject, not degrade to
	// unrestricted.
	if cid := doc.Restriction.Whitelist; cid != "" {
		entries, err := c.fetchPropertiesEntries(cid)
		if err != nil {
			return nil, fmt.Errorf("ResolveNFTProperties: NFT %s: whitelist %s: %w", nftTokenID, cid, err)
		}
		resolved.Whitelist = entries
	}
	if cid := doc.Restriction.Admins; cid != "" {
		entries, err := c.fetchPropertiesEntries(cid)
		if err != nil {
			return nil, fmt.Errorf("ResolveNFTProperties: NFT %s: admins %s: %w", nftTokenID, cid, err)
		}
		resolved.Admins = entries
	}

	return resolved, nil
}

// syncPropertiesChain fetches the properties chain from the NFT's deployer peer.
// It reports only whether a peer was contacted; whether a chain was actually
// applied can be established only by a local re-read.
func (c *Core) syncPropertiesChain(nftTokenID, propsTokenID string) (bool, error) {
	nftInfo, err := c.fetchContractInfo(nftTokenID)
	if err != nil {
		return false, fmt.Errorf("fetching NFT metadata to locate its peer: %w", err)
	}
	if nftInfo.PeerID == "" || nftInfo.DID == "" {
		return false, fmt.Errorf("NFT metadata carries no peer address (peerID=%q did=%q)", nftInfo.PeerID, nftInfo.DID)
	}

	address := nftInfo.PeerID + "." + nftInfo.DID
	if err := c.SyncTransactionChainsFromPeer(address, []string{propsTokenID}, nil, nil, false, false); err != nil {
		return false, fmt.Errorf("syncing properties chain from peer %s: %w", address, err)
	}
	return true, nil
}

// propertiesDocCIDFromChain reads the document CID from the chain tip and the
// deployer from the properties token's own genesis — using the NFT's genesis
// instead would force a sync of a chain we otherwise do not need.
func (c *Core) propertiesDocCIDFromChain(propsTokenID string, tip *models.Transactions) (string, string, error) {
	var tipInfo models.TransactionInfo
	if err := json.Unmarshal(tip.Info, &tipInfo); err != nil {
		return "", "", fmt.Errorf("parsing properties chain tip for %s: %w", propsTokenID, err)
	}

	docCID := ""
	if tipInfo.Tokens != nil {
		for _, t := range tipInfo.Tokens.Properties {
			if t != nil && t.TokenID == propsTokenID {
				docCID = t.Data
				break
			}
		}
	}
	if docCID == "" {
		return "", "", fmt.Errorf("properties chain tip for %s carries no document CID", propsTokenID)
	}
	if err := util.ValidateCIDFormat(docCID); err != nil {
		return "", "", fmt.Errorf("properties chain tip for %s carries an invalid document CID: %w", propsTokenID, err)
	}

	deployer, err := c.w.GetGenesisInitiatorDID(propsTokenID, false)
	if err != nil {
		return "", "", fmt.Errorf("reading deployer of properties token %s: %w", propsTokenID, err)
	}

	return docCID, deployer, nil
}

// fetchPropertiesDocument fetches and parses a properties document.
func (c *Core) fetchPropertiesDocument(cid string) (*models.TokenProperties, error) {
	raw, err := c.catBytes(cid)
	if err != nil {
		return nil, fmt.Errorf("fetching properties document %s: %w", cid, err)
	}
	doc, err := models.ParseTokenProperties(raw)
	if err != nil {
		return nil, fmt.Errorf("properties document %s: %w", cid, err)
	}
	return doc, nil
}

// fetchPropertiesEntries fetches and parses a whitelist or admins document.
func (c *Core) fetchPropertiesEntries(cid string) ([]string, error) {
	raw, err := c.catBytes(cid)
	if err != nil {
		return nil, fmt.Errorf("fetching entries document: %w", err)
	}
	entries, err := models.ParsePropertiesEntries(raw)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// catBytes reads an IPFS object, rejecting non-CIDs before the fetch so Cat
// cannot silently hash the input as content.
func (c *Core) catBytes(cid string) ([]byte, error) {
	// Guard before the fetch: Cat hashes non-CID input as content, silently
	// resolving to an unrelated document instead of erroring.
	if err := util.ValidateCIDFormat(cid); err != nil {
		return nil, err
	}
	reader, err := c.ipfsOps.Cat(cid)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", cid, err)
	}
	return raw, nil
}

// PinPropertiesDocuments pins the properties document and everything it points
// to. Validation fails closed, so a garbage-collected whitelist document would
// make every execute of that NFT fail permanently.
func (c *Core) PinPropertiesDocuments(doc *models.TokenProperties, docCID string) error {
	if doc == nil {
		return fmt.Errorf("PinPropertiesDocuments: document is nil")
	}

	cids := []string{docCID}
	if doc.Restriction.Whitelist != "" {
		cids = append(cids, doc.Restriction.Whitelist)
	}
	if doc.Restriction.Admins != "" {
		cids = append(cids, doc.Restriction.Admins)
	}

	for _, cid := range cids {
		if err := util.ValidateCIDFormat(cid); err != nil {
			return fmt.Errorf("PinPropertiesDocuments: refusing to pin invalid CID: %w", err)
		}
		if err := c.ipfsOps.Pin(cid, nil); err != nil {
			return fmt.Errorf("PinPropertiesDocuments: pinning %s: %w", cid, err)
		}
	}
	return nil
}
