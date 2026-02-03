package rac

type RacBlock struct {
	bb []byte
	bm map[string]interface{}
}

func InitRacBlock(bb []byte, bm map[string]interface{}) (*RacBlock, error) {
	r := &RacBlock{
		bb: bb,
		bm: bm,
	}
	return r, nil
}
