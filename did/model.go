package did

const (
	LiteDIDMode int = 4
)

const (
	BIPVersion int = iota
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
	MnemonicFileName     string = "mnemonic.txt"
)

const (
	DefaultPWD string = "Rubix#PrivKey"
)

type DIDCreate struct {
	PrivPWD   string `json:"priv_pwd"`
	PubKey    string `json:"pub_key"`
	PrivKey   string `json:"priv_key"`
	Mnemonic  string `json:"mnemonic"`
	ChildPath int    `json:"childPath"`
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
