package timeutil

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/rubixchain/rubixgoplatform/wrapper/helper/strutil"
)

func ParseDurationSecond(in interface{}) (time.Duration, error) {
	var dur time.Duration
	jsonIn, ok := in.(json.Number)
	if ok {
		in = jsonIn.String()
	}
	switch inp := in.(type) {
	case nil:
		// return default of zero
	case string:
		if inp == "" {
			return dur, nil
		}
		var err error
		// Look for a suffix otherwise its a plain second value
		if strings.HasSuffix(inp, "s") || strings.HasSuffix(inp, "m") || strings.HasSuffix(inp, "h") || strings.HasSuffix(inp, "ms") {
			dur, err = time.ParseDuration(inp)
			if err != nil {
				return dur, err
			}
		} else {
			// Plain integer
			secs, err := strconv.ParseInt(inp, 10, 64)
			if err != nil {
				return dur, err
			}
			dur = time.Duration(secs) * time.Second
		}
	case int:
		dur = time.Duration(inp) * time.Second
	case int32:
		dur = time.Duration(inp) * time.Second
	case int64:
		dur = time.Duration(inp) * time.Second
	case uint:
		dur = time.Duration(inp) * time.Second
	case uint32:
		dur = time.Duration(inp) * time.Second
	case uint64:
		dur = time.Duration(inp) * time.Second
	case float32:
		dur = time.Duration(inp) * time.Second
	case float64:
		dur = time.Duration(inp) * time.Second
	case time.Duration:
		dur = inp
	default:
		return 0, errors.New("could not parse duration from input")
	}

	return dur, nil
}

func ParseAbsoluteTime(in interface{}) (time.Time, error) {
	var t time.Time
	switch inp := in.(type) {
	case nil:
		// return default of zero
		return t, nil
	case string:
		// Allow RFC3339 with nanoseconds, or without,
		// or an epoch time as an integer.
		var err error
		t, err = time.Parse(time.RFC3339Nano, inp)
		if err == nil {
			break
		}
		t, err = time.Parse(time.RFC3339, inp)
		if err == nil {
			break
		}
		epochTime, err := strconv.ParseInt(inp, 10, 64)
		if err == nil {
			t = time.Unix(epochTime, 0)
			break
		}
		return t, errors.New("could not parse string as date and time")
	case json.Number:
		epochTime, err := inp.Int64()
		if err != nil {
			return t, err
		}
		t = time.Unix(epochTime, 0)
	case int:
		t = time.Unix(int64(inp), 0)
	case int32:
		t = time.Unix(int64(inp), 0)
	case int64:
		t = time.Unix(inp, 0)
	case uint:
		t = time.Unix(int64(inp), 0)
	case uint32:
		t = time.Unix(int64(inp), 0)
	case uint64:
		t = time.Unix(int64(inp), 0)
	default:
		return t, errors.New("could not parse time from input type")
	}
	return t, nil
}

func ParseInt(in interface{}) (int64, error) {
	var ret int64
	jsonIn, ok := in.(json.Number)
	if ok {
		in = jsonIn.String()
	}
	switch in.(type) {
	case string:
		inp := in.(string)
		if inp == "" {
			return 0, nil
		}
		var err error
		left, err := strconv.ParseInt(inp, 10, 64)
		if err != nil {
			return ret, err
		}
		ret = left
	case int:
		ret = int64(in.(int))
	case int32:
		ret = int64(in.(int32))
	case int64:
		ret = in.(int64)
	case uint:
		ret = int64(in.(uint))
	case uint32:
		ret = int64(in.(uint32))
	case uint64:
		ret = int64(in.(uint64))
	default:
		return 0, errors.New("could not parse value from input")
	}

	return ret, nil
}

func ParseBool(in interface{}) (bool, error) {
	var result bool
	if err := mapstructure.WeakDecode(in, &result); err != nil {
		return false, err
	}
	return result, nil
}

func ParseString(in interface{}) (string, error) {
	var result string
	if err := mapstructure.WeakDecode(in, &result); err != nil {
		return "", err
	}
	return result, nil
}

func ParseCommaStringSlice(in interface{}) ([]string, error) {
	rawString, ok := in.(string)
	if ok && rawString == "" {
		return []string{}, nil
	}
	var result []string
	config := &mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToSliceHookFunc(","),
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(in); err != nil {
		return nil, err
	}
	return strutil.TrimStrings(result), nil
}
