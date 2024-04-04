package logger

import (
	"io"
	"os"
	"testing"
)

func TestLogger(t *testing.T) {
	fp, err := os.OpenFile("log.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	l := New(&LoggerOptions{
		Level:  Debug,
		Color:  []ColorOption{AutoColor, ColorOff},
		Output: []io.Writer{DefaultOutput, fp},
	})
	l.Debug("Test")
	l.Info("Test")
}
