package apiconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
)

type APIConfig struct {
	Rand string `json:"rand"`
	Data string `json:"data"`
}

func LoadAPIConfig(fileName string, keyStr string, config interface{}) error {
	configData, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	api := &APIConfig{}
	err = json.Unmarshal(configData, api)
	if err != nil {
		return err
	}
	nonce, err := base64.StdEncoding.DecodeString(api.Rand)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(api.Data)
	if err != nil {
		return err
	}

	h := sha256.New()
	h.Write(nonce)
	h.Write([]byte(keyStr))
	key := h.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	plain, err := aesgcm.Open(nil, nonce, data, nil)
	if err != nil {
		return err
	}

	err = json.Unmarshal(plain, config)
	if err != nil {
		return err
	}
	return nil
}

func CreateAPIConfig(fileName string, keyStr string, config []byte) error {

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	h := sha256.New()
	h.Write(nonce)
	h.Write([]byte(keyStr))
	key := h.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}

	encData := aesgcm.Seal(nil, nonce, config, nil)

	api := APIConfig{
		Rand: base64.StdEncoding.EncodeToString(nonce),
		Data: base64.StdEncoding.EncodeToString(encData),
	}

	data, err := json.Marshal(api)
	if err != nil {
		return err
	}

	fp, err := os.Create(fileName)
	if err != nil {
		return err
	}
	_, err = fp.WriteString(string(data))
	if err != nil {
		return err
	}
	fp.Close()
	return nil
}
