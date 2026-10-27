package logger

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const mib = 1024 * 1024

// lumberjack's backup timestamp layout.
const backupTimeFormat = "2006-01-02T15-04-05.000"

var backupName = regexp.MustCompile(`^log-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3})\.txt\.gz$`)

func TestMain(m *testing.M) {
	// A non-UTC local zone, so a backup name in UTC cannot pass for local time.
	time.Local = time.FixedZone("IST", 5*3600+30*60)
	os.Exit(m.Run())
}

func newTestLog(t *testing.T, maxSize int) (string, *RotatingFile, Logger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log.txt")
	lj := NewRotatingFile(path, maxSize)
	t.Cleanup(func() { lj.Close() })
	log := New(&LoggerOptions{
		Name:   "Main",
		Level:  Debug,
		Color:  []ColorOption{ColorOff},
		Output: []io.Writer{lj},
	})
	return path, lj, log
}

// waitForBackups waits until lumberjack has compressed every rotated file and
// returns the compressed backups.
func waitForBackups(t *testing.T, path string) []string {
	t.Helper()
	dir := filepath.Dir(path)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		pending, _ := filepath.Glob(filepath.Join(dir, "log-*.txt"))
		if len(pending) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backups not compressed: %v", pending)
		}
	}
	backups, err := filepath.Glob(filepath.Join(dir, "log-*.txt.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return backups
}

func readGzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("%s is not gzip: %v", path, err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0 // lumberjack creates the file on the first write
	}
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestLogRotationAtMaxSize(t *testing.T) {
	path, _, log := newTestLog(t, 1)
	payload := strings.Repeat("x", 1000)

	// Fill log.txt to just under 1 MiB: no rotation yet.
	n := 0
	for fileSize(t, path)+2*int64(len(payload)) < mib {
		log.Info(fmt.Sprintf("line-%d %s", n, payload))
		n++
	}
	if backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "log-*")); len(backups) != 0 {
		t.Fatalf("rotated before reaching the max size: %v", backups)
	}

	// Crossing 1 MiB rotates.
	log.Info(fmt.Sprintf("line-%d %s", n, payload))
	log.Info(fmt.Sprintf("line-%d %s", n+1, payload))
	log.Info("after rotation")

	backups := waitForBackups(t, path)
	if len(backups) != 1 {
		t.Fatalf("got backups %v, want exactly one", backups)
	}
	archived := readGzip(t, backups[0])
	if first, _, _ := strings.Cut(archived, "\n"); !strings.Contains(first, " [INFO]  Main: line-0 ") {
		t.Fatalf("backup does not start with the first log line: %.80q", archived)
	}
	if size := len(archived); size > mib {
		t.Fatalf("backup holds %d bytes, more than the 1 MiB max size", size)
	}

	active := readFile(t, path)
	if !strings.Contains(active, "Main: after rotation") || strings.Contains(active, "line-0 ") {
		t.Fatalf("log.txt after rotation = %.200q", active)
	}
	if got := strings.Count(archived+active, "Main: line-"); got != n+2 {
		t.Fatalf("found %d lines across log.txt and the backup, want %d", got, n+2)
	}

	t.Run("backup name uses the log.txt timezone with milliseconds", func(t *testing.T) {
		m := backupName.FindStringSubmatch(filepath.Base(backups[0]))
		if m == nil {
			t.Fatalf("backup %q does not match %s", filepath.Base(backups[0]), backupName)
		}
		rotated, err := time.ParseInLocation(backupTimeFormat, m[1], time.Local)
		if err != nil {
			t.Fatal(err)
		}

		// The last archived line was written just before the rotation.
		lines := strings.Split(strings.TrimSpace(archived), "\n")
		last := strings.Fields(lines[len(lines)-1])[0]
		if !strings.HasSuffix(last, "+0530") {
			t.Fatalf("log line timestamp %q is not in the local zone", last)
		}
		written, err := time.Parse(TimeFormat, last)
		if err != nil {
			t.Fatal(err)
		}
		if d := rotated.Sub(written); d < 0 || d > 5*time.Second {
			t.Fatalf("backup time %s is %s after last log line %s, want the same wall clock", m[1], d, last)
		}
	})
}

func TestLogRotationKeepsAllBackups(t *testing.T) {
	path, lj, log := newTestLog(t, 1)
	dir := filepath.Dir(path)

	// An old backup from a previous run must survive later rotations.
	old := filepath.Join(dir, "log-2000-01-01T00-00-00.000.txt.gz")
	f, err := os.Create(old)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	io.WriteString(zw, "from a previous run\n")
	zw.Close()
	f.Close()
	longAgo := time.Now().AddDate(-20, 0, 0)
	if err := os.Chtimes(old, longAgo, longAgo); err != nil {
		t.Fatal(err)
	}

	const rotations = 5
	for i := 0; i < rotations; i++ {
		log.Info(fmt.Sprintf("generation-%d", i))
		if err := lj.Rotate(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond) // distinct millisecond in each backup name
	}

	backups := waitForBackups(t, path)
	if len(backups) != rotations+1 {
		t.Fatalf("got %d backups %v, want %d", len(backups), backups, rotations+1)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("old backup was deleted: %v", err)
	}
	all := ""
	for _, b := range backups {
		all += readGzip(t, b)
	}
	for i := 0; i < rotations; i++ {
		if !strings.Contains(all, fmt.Sprintf("Main: generation-%d\n", i)) {
			t.Fatalf("generation-%d missing from backups", i)
		}
	}
}

func TestLogRotationConcurrentWrites(t *testing.T) {
	path, _, log := newTestLog(t, 1)
	payload := strings.Repeat("y", 300)

	const writers, lines = 8, 1500 // ~3.6 MiB, several rotations
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < lines; i++ {
				log.Info(fmt.Sprintf("msg-%d-%d %s", w, i, payload))
			}
		}(w)
	}
	wg.Wait()

	backups := waitForBackups(t, path)
	if len(backups) < 3 {
		t.Fatalf("expected several rotations, got %d", len(backups))
	}
	all := readFile(t, path)
	for _, b := range backups {
		all += readGzip(t, b)
	}
	// Every line is whole: no interleaving or truncation across rotations.
	sc := bufio.NewScanner(strings.NewReader(all))
	count := 0
	for sc.Scan() {
		if !strings.HasSuffix(sc.Text(), " "+payload) {
			t.Fatalf("broken log line %.80q", sc.Text())
		}
		count++
	}
	if count != writers*lines {
		t.Fatalf("found %d log lines across log.txt and backups, want %d", count, writers*lines)
	}
}
