package command

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func writeLogs(t *testing.T, path string, rotation bool, maxSize int, msgs ...string) {
	t.Helper()
	fp, err := openLogFile(path, rotation, maxSize)
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	log := logger.New(&logger.LoggerOptions{
		Name:   "Main",
		Level:  logger.Debug,
		Color:  []logger.ColorOption{logger.ColorOff}, // as Run sets for the log file
		Output: []io.Writer{fp},
	})
	for _, m := range msgs {
		log.Info(m)
	}
	log.Debug("debug line")
	log.Error("error line", "err", "boom")
}

var timestamp = regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4} `)

func TestOpenLogFileSameFormat(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.txt")
	rotating := filepath.Join(dir, "rotating.txt")
	writeLogs(t, plain, false, 30, "hello")
	writeLogs(t, rotating, true, 30, "hello")

	read := func(p string) string {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	want, got := read(plain), read(rotating)
	if n := len(timestamp.FindAllString(want, -1)); n != 3 {
		t.Fatalf("plain log has %d timestamped lines, want 3:\n%s", n, want)
	}
	if timestamp.ReplaceAllString(got, "") != timestamp.ReplaceAllString(want, "") {
		t.Fatalf("rotating log format differs from plain log:\nplain:\n%s\nrotating:\n%s", want, got)
	}
}

func TestOpenLogFileRotationKeepsMode(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.txt")
	rotDir := filepath.Join(dir, "rotating")
	if err := os.Mkdir(rotDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rotDir, "log.txt")

	writeLogs(t, plain, false, 1, "hello")
	// A new log.txt filled past 1 MiB, so it rotates.
	msgs := make([]string, 1200)
	for i := range msgs {
		msgs[i] = strings.Repeat("m", 1000)
	}
	writeLogs(t, path, true, 1, msgs...)

	mode := func(p string) os.FileMode {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode()
	}
	want := mode(plain) // 0644 less the umask
	var backups []string
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		pending, _ := filepath.Glob(filepath.Join(rotDir, "log-*.txt"))
		backups, _ = filepath.Glob(filepath.Join(rotDir, "log-*.txt.gz"))
		if len(pending) == 0 && len(backups) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no compressed backup: pending %v", pending)
		}
	}
	for _, p := range append(backups, path) {
		if got := mode(p); got != want {
			t.Errorf("%s has mode %v, want %v", filepath.Base(p), got, want)
		}
	}
}

func TestOpenLogFileRotationDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")

	// Well past a 1 MiB max size, over two opens: nothing is rotated.
	msg := strings.Repeat("z", 1000)
	msgs := make([]string, 800)
	for i := range msgs {
		msgs[i] = msg
	}
	writeLogs(t, path, false, 1, msgs...)
	writeLogs(t, path, false, 1, msgs...)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only log.txt, got %v", entries)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), msg); got != 2*len(msgs) {
		t.Fatalf("log.txt has %d messages, want %d", got, 2*len(msgs))
	}
}
