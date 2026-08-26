package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/rubixchain/rubixgoplatform/constants"
	rubixmath "github.com/rubixchain/rubixgoplatform/math"
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

	// Always sync, even when a chain is already held: properties are mutable,
	// so a cached chain may be behind and would otherwise enforce a superseded
	// document forever. applyTokenChainFromSync appends only entries past the
	// local prefix and rejects forks, so re-syncing a current chain is a no-op.
	synced, syncErr := c.syncPropertiesChain(nftTokenID, propsTokenID)
	if !synced {
		if tx == nil {
			// Nothing cached and no peer reachable, so "no properties" and
			// "could not check" are indistinguishable; either guess fails open.
			return nil, fmt.Errorf("ResolveNFTProperties: cannot determine whether NFT %s has properties: %w",
				nftTokenID, syncErr)
		}
		// A cached chain is better than nothing: enforce it rather than
		// rejecting every transaction whenever the peer is briefly unreachable.
		// It may be stale, but it is signed and was valid when fetched.
		c.log.Warn("ResolveNFTProperties: peer unreachable; enforcing possibly-stale cached properties",
			"nft", nftTokenID, "propertiesToken", propsTokenID, "err", syncErr)
	}

	// Re-read locally: SyncTransactionChainsFromPeer returns nil even when the
	// apply failed, so only the local tip proves anything. Without this a failed
	// sync reads as "no properties" and the gate fails open.
	tx, role, err = c.w.GetLatestTransactionAndRoleByTokenID(propsTokenID)
	if err != nil {
		return nil, fmt.Errorf("ResolveNFTProperties: re-reading chain tip for %s after sync: %w", propsTokenID, err)
	}
	if tx == nil {
		// The peer answered and has no properties chain for this NFT — the
		// legitimate unrestricted case.
		c.log.Debug("ResolveNFTProperties: no properties token after sync; treating NFT as unrestricted",
			"nft", nftTokenID, "propertiesToken", propsTokenID)
		return nil, nil
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

// BuildPropertiesToken creates or updates the properties token governing
// nftTokenID, uploading the document and the whitelist/admins lists to IPFS and
// pinning all of them. It returns the token entry to place in the transaction.
//
// The deployer is bootstrapped as the first admin on creation.
func (c *Core) BuildPropertiesToken(nftTokenID string, info *models.PropertiesInfo, initiator string) (*models.TokenInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("BuildPropertiesToken: properties are required when setProperties is true")
	}

	propsTokenID, err := c.GetPropertiesTokenID(nftTokenID)
	if err != nil {
		return nil, err
	}

	doc := info.ToDocument()

	// The deployer is always an admin, so an edit is always possible by
	// someone even if the caller omits the list.
	admins := info.Admins
	if len(admins) == 0 {
		admins = []string{initiator}
	} else if !slices.Contains(admins, initiator) {
		admins = append([]string{initiator}, admins...)
	}
	adminsCID, err := c.uploadPropertiesEntries(admins)
	if err != nil {
		return nil, fmt.Errorf("BuildPropertiesToken: uploading admins: %w", err)
	}
	doc.Restriction.Admins = adminsCID

	if len(info.Whitelist) > 0 {
		whitelistCID, err := c.uploadPropertiesEntries(info.Whitelist)
		if err != nil {
			return nil, fmt.Errorf("BuildPropertiesToken: uploading whitelist: %w", err)
		}
		doc.Restriction.Whitelist = whitelistCID
	}

	raw, err := doc.Serialize()
	if err != nil {
		return nil, fmt.Errorf("BuildPropertiesToken: %w", err)
	}
	docCID, err := c.ipfsOps.Add(bytes.NewReader(raw), nil)
	if err != nil {
		return nil, fmt.Errorf("BuildPropertiesToken: uploading properties document: %w", err)
	}

	if err := c.PinPropertiesDocuments(doc, docCID); err != nil {
		return nil, err
	}

	// An existing chain means this is an edit, so the tip becomes the previous
	// transaction; absent means genesis.
	previousTxID := ""
	if tx, _, err := c.w.GetLatestTransactionAndRoleByTokenID(propsTokenID); err == nil && tx != nil {
		previousTxID = tx.ID
	}

	return &models.TokenInfo{
		TokenID:               propsTokenID,
		PreviousTransactionID: previousTxID,
		Data:                  docCID,
		TokenValue:            rubixmath.MinDecimalUnit(),
	}, nil
}

// uploadPropertiesEntries stores a whitelist or admins list in IPFS.
func (c *Core) uploadPropertiesEntries(entries []string) (string, error) {
	raw, err := json.Marshal(models.PropertiesEntries{Entries: entries})
	if err != nil {
		return "", err
	}
	return c.ipfsOps.Add(bytes.NewReader(raw), nil)
}

// GetNFTProperties resolves the properties governing an NFT for the read API.
func (c *Core) GetNFTProperties(nftTokenID string) (*models.ResolvedProperties, error) {
	return c.ResolveNFTProperties(nftTokenID)
}

// validatePropertiesRequest checks a setProperties request before the build
// locks any NFT. It cannot be combined with a burn or with value transfer, and
// must name exactly one NFT since properties govern a single token.
func (c *Core) validatePropertiesRequest(request *models.TransactionRequest) error {
	if request.Tokens.Properties == nil {
		return fmt.Errorf("setProperties requires a properties object")
	}
	if request.Tokens.BurnNFT {
		return fmt.Errorf("setProperties cannot be combined with burnNft")
	}
	if request.Tokens.RBT > 0 || len(request.Tokens.FT) > 0 || len(request.Tokens.SmartContract) > 0 {
		return fmt.Errorf("setProperties cannot be combined with RBT, FT or smart contract transfers")
	}

	// Multi-NFT setProperties is intended but not yet implemented: the wire
	// format already carries an array and the validator already handles N, so
	// lifting this means looping the build and caching resolution to avoid an
	// O(N^2) peer-sync in validatePropertiesEdits.
	nfts := request.GetAllNFTs()
	if len(nfts) != 1 {
		return fmt.Errorf("setProperties currently requires exactly one NFT, got %d", len(nfts))
	}
	if nfts[0].ParentNFTId != "" {
		return fmt.Errorf("setProperties cannot be combined with child-minting")
	}

	// Reject a malformed document now rather than after the NFTs are locked.
	if err := request.Tokens.Properties.ToDocument().Validate(); err != nil {
		return fmt.Errorf("setProperties: %w", err)
	}

	// An edit is deployer-only, enforced again at consensus. Checking here
	// gives the caller a clear error instead of a quorum rejection.
	resolved, err := c.ResolveNFTProperties(nfts[0].NFTId)
	if err != nil {
		return fmt.Errorf("setProperties: %w", err)
	}
	if resolved != nil && resolved.Deployer != "" && resolved.Deployer != request.Initiator {
		return fmt.Errorf("setProperties: only the deployer %s may edit these properties", resolved.Deployer)
	}

	return nil
}
