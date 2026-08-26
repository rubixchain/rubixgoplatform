package util

import "testing"

// ValidateCIDFormat guards every ipfsOps.Cat call on the properties path.
// Anything it wrongly accepts gets hashed as content by Cat and silently
// resolves to an unrelated document instead of erroring.
func TestValidateCIDFormatRejectsNonCIDs(t *testing.T) {
	const validCID = "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco"

	if err := ValidateCIDFormat(validCID); err != nil {
		t.Errorf("expected %q to be accepted: %v", validCID, err)
	}

	invalid := []string{
		"",
		"Qm",
		"QmTooShort",
		"just-a-string",
		"../etc/passwd",
		validCID + "/subpath",
		validCID + "?query=1",
		validCID + "#fragment",
		"\n" + validCID,
		"bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi",
	}
	for _, s := range invalid {
		if err := ValidateCIDFormat(s); err == nil {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}
