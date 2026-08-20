package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// TokenProperties is the permission document governing a token, stored in IPFS.
// Parsing is fail-closed: bad input errors rather than reading as unrestricted,
// because a gate that degrades to permissive on malformed input is not a gate.
type TokenProperties struct {
	Version     int                `json:"v"`
	Flags       uint32             `json:"f"`
	Policy      PropertiesPolicy   `json:"p"`
	Restriction PropertiesRestrict `json:"r"`
}

// PropertiesPolicy carries time bounds in Unix seconds, 0 meaning unbounded.
// Compared against txnInfo.Epoch, not wall clock, so every node agrees.
type PropertiesPolicy struct {
	ValidFrom int64 `json:"valid_from"`
	ValidTo   int64 `json:"valid_to"`
}

// PropertiesRestrict holds the restrictions; Whitelist and Admins are CIDs so a
// large list does not bloat the document. Each field is independently optional
// (absent means unrestricted); present restrictions combine as AND.
type PropertiesRestrict struct {
	Whitelist             string   `json:"whitelist"`
	Admins                string   `json:"admins"`
	AllowedSubnets        []string `json:"allowed_subnets"`
	AllowedSmartContracts []string `json:"allowed_smart_contracts"`
}

// PropertiesDocVersion is the only version this build understands. Adding a
// flag or field means a new version; unrecognised versions are rejected.
const PropertiesDocVersion = 1

// Flag bits for the f bitfield. Only transferable is defined for v1.
const (
	// FlagTransferable permits transfer. When unset transfer is blocked
	// absolutely — the whitelist does not exempt it.
	FlagTransferable uint32 = 1 << 0

	// KnownFlagsV1 is every bit defined at v1. Other bits are rejected, since
	// ignoring them would fail open on a deliberately set restriction.
	KnownFlagsV1 = FlagTransferable
)

// PropertiesEntries is the shape of the Whitelist and Admins documents. A
// slice, never a map, so json.Marshal output stays deterministic.
type PropertiesEntries struct {
	Entries []string `json:"entries"`
}

// ResolvedProperties is a properties document with its CID-pointed documents
// already dereferenced, so callers never fetch during validation.
type ResolvedProperties struct {
	PropertiesTokenID string
	DocCID            string
	Doc               *TokenProperties
	// Whitelist and Admins are the dereferenced CID contents; nil means the
	// CID was absent, i.e. unrestricted for that field alone.
	Whitelist []string
	Admins    []string
	// Deployer is the properties token's genesis initiator — the only DID
	// permitted to edit the admins list.
	Deployer string
}

// IsTransferable reports whether the transferable flag is set.
func (p *TokenProperties) IsTransferable() bool {
	return p.Flags&FlagTransferable != 0
}

// IsWithinValidityWindow reports whether epoch falls inside the policy window;
// a zero bound is unbounded on that side.
func (p *TokenProperties) IsWithinValidityWindow(epoch int64) bool {
	if p.Policy.ValidFrom != 0 && epoch < p.Policy.ValidFrom {
		return false
	}
	if p.Policy.ValidTo != 0 && epoch > p.Policy.ValidTo {
		return false
	}
	return true
}

// ParseTokenProperties decodes and validates a properties document. Any error
// means the caller must reject the transaction, not treat it as unrestricted.
func ParseTokenProperties(raw []byte) (*TokenProperties, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("properties document is empty")
	}

	// Unknown fields error rather than being silently dropped, matching the
	// fail-closed stance taken for unknown flag bits.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var doc TokenProperties
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("properties document is malformed: %w", err)
	}

	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Validate enforces the version gate, flag-bit gate, and field well-formedness,
// all fail-closed.
func (p *TokenProperties) Validate() error {
	if p.Version != PropertiesDocVersion {
		return fmt.Errorf("unsupported properties document version %d (this build understands %d)",
			p.Version, PropertiesDocVersion)
	}

	if unknown := p.Flags &^ KnownFlagsV1; unknown != 0 {
		return fmt.Errorf("properties document sets unknown flag bits 0x%x at version %d",
			unknown, p.Version)
	}

	if p.Policy.ValidFrom < 0 || p.Policy.ValidTo < 0 {
		return fmt.Errorf("properties validity bounds must not be negative (from=%d to=%d)",
			p.Policy.ValidFrom, p.Policy.ValidTo)
	}
	if p.Policy.ValidFrom != 0 && p.Policy.ValidTo != 0 && p.Policy.ValidFrom > p.Policy.ValidTo {
		return fmt.Errorf("properties validity window is inverted (from=%d > to=%d)",
			p.Policy.ValidFrom, p.Policy.ValidTo)
	}

	return nil
}

// Serialize encodes the document for storage in IPFS.
func (p *TokenProperties) Serialize() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("refusing to serialize an invalid properties document: %w", err)
	}
	return json.Marshal(p)
}

// ParsePropertiesEntries decodes a whitelist or admins document, failing closed
// like the properties document itself.
func ParsePropertiesEntries(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("entries document is empty")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var doc PropertiesEntries
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("entries document is malformed: %w", err)
	}
	for i, e := range doc.Entries {
		if strings.TrimSpace(e) == "" {
			return nil, fmt.Errorf("entries document has an empty entry at index %d", i)
		}
	}
	return doc.Entries, nil
}

// NFTPropertiesResponse is the read-API shape, flattened from the compact
// document so callers do not have to decode the flag bitfield themselves.
type NFTPropertiesResponse struct {
	NFTId                 string   `json:"nftId"`
	PropertiesTokenID     string   `json:"propertiesTokenId"`
	PropertiesCID         string   `json:"propertiesCid"`
	Version               int      `json:"version"`
	Transferable          bool     `json:"transferable"`
	ValidFrom             int64    `json:"validFrom"`
	ValidTo               int64    `json:"validTo"`
	Whitelist             []string `json:"whitelist"`
	Admins                []string `json:"admins"`
	AllowedSubnets        []string `json:"allowedSubnets"`
	AllowedSmartContracts []string `json:"allowedSmartContracts"`
	Deployer              string   `json:"deployer"`
}
