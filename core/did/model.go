package did

const (
	BasicDIDMode int = iota
	StandardDIDMode
	WalletDIDMode
)

const (
	ImgFileName      string = "image.png"
	DIDImgFileName   string = "did.png"
	PubShareFileName string = "pubShare.png"
	PubKeyFileName   string = "pubKey.pem"
)

const (
	DefaultPWD string = "Rubix#PrivKey"
)

type DIDCreate struct {
	Type       int                    `json:"type"`
	Config     map[string]interface{} `json:"config"`
	Secret     string                 `json:"secret"`
	PrivPWD    string                 `json:"priv_pwd"`
	QuorumPWD  string                 `json:"quorum_pwd"`
	ImgFile    string                 `json:"img_file"`
	DIDImgFile string                 `json:"did_img_file"`
	PubImgFile string                 `json:"pub_img_file"`
	PubKeyFile string                 `json:"pub_key_file"`
}
