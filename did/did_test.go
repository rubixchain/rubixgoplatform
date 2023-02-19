package did

// func TestBasic(t *testing.T) {
// 	var dc DIDCrypto

// 	dc := DIDChan{

// 	}

// 	dc = InitDIDBasic("bafybmifa7to5hxicjwehml6aaqqekv3nveusbpblso5md6coddjqyomqii", "./", "mypassword")

// 	msg := "TestMessage"

// 	h := util.CalculateHash([]byte(msg), "SHA3-256")

// 	shareSig, keySig, err := dc.Sign(hex.EncodeToString(h))
// 	if err != nil {
// 		t.Fatalf("Failed to create signature : %s", err.Error())
// 	}
// 	b, err := dc.Verify(hex.EncodeToString(h), shareSig, keySig)
// 	if err != nil {
// 		t.Fatalf("Failed to verify signature : %s", err.Error())
// 	}
// 	if !b {
// 		t.Fatal("Failed to verify singature")
// 	}
// }
