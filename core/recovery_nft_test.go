package core

import (
	"testing"

	"github.com/rubixchain/rubixgoplatform/util"
)

func TestEphemeralNFTID(t *testing.T) {
	const nftID = "QmbKP6dLYpD8LH9bjjztQGFMhpQULBZYZkCxVSwTqpAhPm"
	const txID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	got, err := EphemeralNFTID(nftID, txID)
	if err != nil {
		t.Fatal(err)
	}
	if want := nftID[:22] + txID[40:]; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if err := util.ValidateCIDFormat(got); err != nil {
		t.Fatalf("ephemeral NFT ID is not a valid CID: %v", err)
	}

	if _, err := EphemeralNFTID(nftID, txID[:63]); err == nil {
		t.Fatal("expected an error for a short transaction ID")
	}
	if _, err := EphemeralNFTID("bad", txID); err == nil {
		t.Fatal("expected an error for a bad recovery NFT ID")
	}
}
