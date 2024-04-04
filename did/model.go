package did

const (
	BasicDIDMode int = iota
	StandardDIDMode
	WalletDIDMode
	ChildDIDMode
)

const (
	ImgFileName          string = "image.png"
	DIDImgFileName       string = "did.png"
	MasterDIDFileName    string = "master.txt"
	PvtShareFileName     string = "pvtShare.png"
	PubShareFileName     string = "pubShare.png"
	PvtKeyFileName       string = "pvtKey.pem"
	PubKeyFileName       string = "pubKey.pem"
	QuorumPvtKeyFileName string = "quorumPrivKey.pem"
	QuorumPubKeyFileName string = "quorumPubKey.pem"
)

const (
	DefaultPWD string = "Rubix#PrivKey"
)

type DIDCreate struct {
	Type              int    `json:"type"`
	Dir               string `json:"dir"`
	Config            string `json:"config"`
	RootDID           bool   `json:"root_did"`
	MasterDID         string `json:"master_did"`
	Secret            string `json:"secret"`
	PrivPWD           string `json:"priv_pwd"`
	QuorumPWD         string `json:"quorum_pwd"`
	ImgFile           string `json:"img_file"`
	DIDImgFileName    string `json:"did_img_file"`
	PubImgFile        string `json:"pub_img_file"`
	PrivImgFile       string `json:"priv_img_file"`
	PubKeyFile        string `json:"pub_key_file"`
	PrivKeyFile       string `json:"priv_key_file"`
	QuorumPubKeyFile  string `json:"quorum_pub_key_file"`
	QuorumPrivKeyFile string `json:"quorum_priv_key_file"`
}

type DIDSignature struct {
	Pixels    []byte
	Signature []byte
}

type SignReqData struct {
	ID          string `json:"id"`
	Mode        int    `json:"mode"`
	Hash        []byte `json:"hash"`
	OnlyPrivKey bool   `json:"only_priv_key"`
}

type SignRespData struct {
	ID        string       `json:"id"`
	Mode      int          `json:"mode"`
	Password  string       `json:"password"`
	Signature DIDSignature `json:"signature"`
}

// BootStrapResponse used as model for the API responses
type SignResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  SignReqData `json:"result"`
}
