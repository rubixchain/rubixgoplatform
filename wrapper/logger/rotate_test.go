package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLog(t *testing.T) (string, *RotatingFile, Logger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log.txt")
	lj := NewRotatingFile(path)
	log := New(&LoggerOptions{
		Name:   "Main",
		Level:  Debug,
		Color:  []ColorOption{ColorOff},
		Output: []io.Writer{lj},
	})
	return path, lj, log
}

// finishArchives waits until lumberjack has compressed every rotated file and
// they have all been renamed, then closes the log.
func finishArchives(t *testing.T, path string, lj *RotatingFile, log Logger) {
	t.Helper()
	waitFor(t, func() bool { return !archiveRotatedLogs(path, log) })
	lj.Close()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
	}
}

func archives(t *testing.T, path string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "log_*.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return files
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

func TestLogRotationAfterPeriod(t *testing.T) {
	path, lj, log := newTestLog(t)
	log.Info("before rotation")

	stop := StartRotation(lj, 500*time.Millisecond, log)
	time.Sleep(200 * time.Millisecond)
	if len(archives(t, path)) != 0 {
		t.Fatal("rotated before the period elapsed")
	}
	waitFor(t, func() bool { return len(archives(t, path)) > 0 })
	stop()
	log.Info("after rotation")
	finishArchives(t, path, lj, log)

	today := time.Now().Format(archiveDateFormat)
	first := filepath.Join(filepath.Dir(path), fmt.Sprintf("log_%s_%s.gz", today, today))
	if got := readGzip(t, first); !strings.Contains(got, "before rotation") {
		t.Fatalf("archive missing pre-rotation log, got %q", got)
	}
	active := readFile(t, path)
	if !strings.Contains(active, "Log file rotated") || !strings.Contains(active, "after rotation") {
		t.Fatalf("log.txt missing post-rotation logs, got %q", active)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "log-*")); len(leftovers) != 0 {
		t.Fatalf("unrenamed lumberjack backups: %v", leftovers)
	}
}

func TestLogRotationRestart(t *testing.T) {
	t.Run("overdue period rotates with the original start date", func(t *testing.T) {
		path, lj, log := newTestLog(t)
		started := time.Now().AddDate(0, 0, -7)
		line := started.Format(TimeFormat) + " [INFO]  Main: from previous run\n"
		if err := os.WriteFile(path, []byte(line), 0644); err != nil {
			t.Fatal(err)
		}

		stop := StartRotation(lj, 7*24*time.Hour, log)
		waitFor(t, func() bool { return len(archives(t, path)) > 0 })
		stop()
		finishArchives(t, path, lj, log)

		want := fmt.Sprintf("log_%s_%s.gz", started.Format(archiveDateFormat), time.Now().Format(archiveDateFormat))
		got := archives(t, path)
		if len(got) != 1 || filepath.Base(got[0]) != want {
			t.Fatalf("archives = %v, want [%s]", got, want)
		}
		if !strings.Contains(readGzip(t, got[0]), "from previous run") {
			t.Fatal("archive missing previous run's log")
		}
	})

	t.Run("running period is not reset", func(t *testing.T) {
		path, lj, log := newTestLog(t)
		// Started 700ms ago with a 1s period: rotation is due in ~300ms, not 1s.
		line := time.Now().Add(-700*time.Millisecond).Format(TimeFormat) + " [INFO]  Main: from previous run\n"
		if err := os.WriteFile(path, []byte(line), 0644); err != nil {
			t.Fatal(err)
		}

		stop := StartRotation(lj, time.Second, log)
		time.Sleep(700 * time.Millisecond)
		stop()
		finishArchives(t, path, lj, log)

		if len(archives(t, path)) != 1 {
			t.Fatalf("expected one rotation within the remaining period, got %v", archives(t, path))
		}
	})
}

func TestLogRotationConcurrentWrites(t *testing.T) {
	path, lj, log := newTestLog(t)
	stop := StartRotation(lj, 30*time.Millisecond, log)

	const writers, lines = 8, 300
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < lines; i++ {
				log.Info(fmt.Sprintf("msg-%d-%d", w, i))
				time.Sleep(100 * time.Microsecond)
			}
		}(w)
	}
	wg.Wait()
	stop()
	finishArchives(t, path, lj, log)

	all := readFile(t, path)
	files := archives(t, path)
	for _, f := range files {
		all += readGzip(t, f)
	}
	if len(files) < 2 {
		t.Fatalf("expected several rotations, got %d", len(files))
	}
	if got := strings.Count(all, "Main: msg-"); got != writers*lines {
		t.Fatalf("found %d log lines across log.txt and archives, want %d", got, writers*lines)
	}
}
