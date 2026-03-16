package util

import "time"

func GetCurrentTimestamp() time.Time {
	return time.Now()
}

func GetCurrentTimeInUnix() int64 {
	return time.Now().Unix()
}