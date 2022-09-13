package util

import (
	"os"
	"testing"
)

func TestBasic(t *testing.T) {
	err := os.Mkdir("test", 0755)
	if err != nil {
		t.Fatal("Failed to create directory")
	}
	f, err := os.Create("test/test.tx")
	if err != nil {
		t.Fatal("failed to create file")
	}
	f.WriteString("Test write")
	f.Close()
	err = os.Mkdir("temp", 0755)
	if err != nil {
		t.Fatal("Failed to create directory")
	}
	err = DirCopy("test", "temp")
	if err != nil {
		t.Fatal("Failed to copy directory")
	}
}
