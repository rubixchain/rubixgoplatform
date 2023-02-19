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

func (ob *OldBlock) GetHeight() uint64 {
	return uint64(len(ob.blks))
}

func (ob *OldBlock) GetStakedToken() string {
	mi, ok := ob.blks[0][OTCMinIDKey]
	if !ok {
		return ""
	}
	m, ok := mi.([]map[string]interface{})
	if !ok {
		return ""
	}
	sti, ok := m[0][OTCStakedTokenKey]
	if !ok {
		return ""
	}
	st, ok := sti.(string)
	if !ok {
		return ""
	}
	return st
}
