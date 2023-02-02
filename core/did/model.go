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
	Type       int    `json:"type"`
	Dir        string `json:"dir"`
	Config     string `json:"config"`
	Secret     string `json:"secret"`
	PrivPWD    string `json:"priv_pwd"`
	QuorumPWD  string `json:"quorum_pwd"`
	ImgFile    string `json:"img_file"`
	DIDImgFile string `json:"did_img_file"`
	PubImgFile string `json:"pub_img_file"`
	PubKeyFile string `json:"pub_key_file"`
}

type DIDSignature struct {
	Pixels    []byte
	Signature []byte
}

type SignReqData struct {
	ID   string `json:"id"`
	Mode int    `json:"mode"`
}

type SignRespData struct {
	ID       string `json:"id"`
	Mode     int    `json:"mode"`
	Password string `json:"password"`
}

// BootStrapResponse used as model for the API responses
type SignResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  SignReqData `json:"result"`
}
