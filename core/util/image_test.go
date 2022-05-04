package util

import (
	"testing"
)

func TestDID(t *testing.T) {
	pixels, err := CreateDID("TestData", "TestPeerID", "image.png")
	if err != nil {
		t.Fatal("failed to create DID")
	}
	err = CreatePNGImage(pixels, 256, 256, "DID.png")
	if err != nil {
		t.Fatal("failed to create DID image")
	}

	// shares := nlss.GenShares(pixels, nlss.ShareTwo1x2)
	// s1 := shares.Share[0]
	// s2 := shares.Share[1]

	// if len(s1.ShareBytes) == 0 || len(s2.ShareBytes) == 0 {
	// 	t.Fatal("failed to create DID image")
	// }

	// err = CreatePNGImage(s1.ShareBytes, 1024, 512, "private_share.png")
	// if err != nil {
	// 	t.Fatal("failed to create DID image")
	// }
	// err = CreatePNGImage(s2.ShareBytes, 1024, 512, "public_share.png")
	// if err != nil {
	// 	t.Fatal("failed to create DID image")
	// }
}
