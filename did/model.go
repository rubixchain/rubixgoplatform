package did

const (
	DefaultPWD string = "Rubix#PrivKey"
)

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
