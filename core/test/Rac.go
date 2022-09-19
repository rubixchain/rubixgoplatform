package main

import (
	"encoding/json"
	"fmt"
)

type RacApiInput struct {
	Type                  int                    `json:"type"`
	CreatorDID            string                 `json:"creatorDid"`
	TotalSupply           int                    `json:"totalSupply"`
	Hash                  string                 `json:"hash"`
	URL                   string                 `json:"url,omitempty"`
	PvtKeySignature       string                 `json:"pvtKeySignature"`
	CreatorInput          map[string]interface{} `json:"creatorInput"`
	CreatorPubKeyIpfsHash string                 `json:"creatorPubKeyIpfsHash"`
	PvtKey                string                 `json:"pvtKey,omitempty"`
	PvtKeyPass            string                 `json:"pvtKeyPass"`
}

type Rac struct {
	Type            int                    `json:"type"`
	CreatorDID      string                 `json:"creatorDid"`
	TotalSupply     int                    `json:"totalSupply"`
	TokenNumber     int                    `json:"tokenNumber"`
	Hash            string                 `json:"hash"`
	URL             string                 `json:"url,omitempty"`
	PvtKeySignature string                 `json:"pvtKeySignature"`
	CreatorInput    map[string]interface{} `json:"creatorInput"`
}

type RacGensysObject struct {
	CreatorDID            string `json:"creatorDid"`
	Role                  string `json:"creator"`
	CreatorPubKeyIpfsHash string `json:"creatorPubKeyIpfsHash"`
	NftOwner              string `json:"nftOwner"`
	CreatorSign           string `json:"creatorSign"`
}

type RacTokenChain map[string]interface {
}

func ex() {
	x := `{ "type": 2,"creatorPubKeyIpfsHash": "QmYbH5ddZD7hqpth9MWA1T5Kfh4o6g4wYfC5zXkurgYjQd","totalSupply": 2, "contentHash":"acaf10d44fdc2919cc315f124f52a638b5168cdadd0da259e1b0cca1e815b642", "url": "https://drive.google.com/file/d/1oQOdbdeRIV8XwzQzITgrSMcgcGJRhz10/view?usp=sharing", "pvtKeyPass": "jupiterMetaWallet1EcDSA", "pvtKey": "-----BEGIN EC PRIVATE KEY-----\\nProc-Type: 4,ENCRYPTED\\nDEK-Info: AES-128-CBC,92B81148ACAA0CD253733714DDA35A79\\n\\nIJ7E4zfAKJJZle0U9WSnrW0oHJUPnzDZsyf9SeMxFK3z/OyovuiQxT4ozjDSanKX\\nS5/s/gDuhA0Ge8gmcghG91M81LYZIIRgCgW9PuoH2q3YaycDwJbeu6xaj1FyvfZq\\nDKaSVDrWvMEaJmslKQi0Se4mTBYQRnxAywdGydduEuI=\\n-----END EC PRIVATE KEY-----\\n", "creatorInput": { "creatorName": "rubixstagingesdswallet12334", "createdOn": "Thu Jun 30 17:59:14 IST 2022", "nftTitle": "mountain wallpaper", "blockChain": "Rubix", "nftType": "Image", "description": "ttest Eismeer nft merge","color":"black midnight blue and white","comment": "test lambada-0.36 jar"}}`

	var racInput RacApiInput

	json.Unmarshal([]byte(x), &racInput)

	fmt.Println("racInput ::: ", racInput.CreatorInput)

	y := racInput.CreatorInput

	y["creatorPubKeyIpfsHash"] = "QmYbH5ddZD7hqpth9MWA1T5Kfh4o6g4wYfC5zXkurgYjQd"

	fmt.Println("\n adding key to cin ::: ", y)

	//var z map[string]interface{}

	//json.Unmarshal(racInput.CreatorInput, &z)

	//var a enscrypt.PublicKey

}
