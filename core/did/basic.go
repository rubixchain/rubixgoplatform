package did

// DIDBasic will handle basic DID
type DIDBasic struct {
	did  string
	path string
	pwd  string
}

// InitDIDBasic will return the basic did handle
func InitDIDBasic(did string, path string, pwd string) *DIDBasic {
	return &DIDBasic{did: did, path: path, pwd: pwd}
}

// Sign will return the singature of the DID
func (d *DIDBasic) Sign(coord []int) (*DIDSignature, error) {
	// pf := util.SanitizeDirPath(d.path) + d.did + "/" + PvtShareImgFile
	// f, err := os.Open(pf)
	// if err != nil {
	// 	return nil, err
	// }
	// defer f.Close()
	// img, _, err := image.Decode(f)
	// img.
	return nil, nil
}

// Sign will verifyt he signature
func (d *DIDBasic) Verify(coord []int, didSig *DIDSignature) (bool, error) {
	return false, nil
}
