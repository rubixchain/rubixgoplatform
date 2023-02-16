package block

import (
	"encoding/json"
	"io/ioutil"
)

type OldBlock struct {
	blks []map[string]interface{}
}

func InitOldBlock(file string) (*OldBlock, error) {
	rb, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	ob := &OldBlock{
		blks: make([]map[string]interface{}, 0),
	}
	err = json.Unmarshal(rb, &ob.blks)
	if err != nil {
		return nil, err
	}
	return ob, nil
}
