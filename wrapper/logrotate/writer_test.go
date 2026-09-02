package logrotate

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestWriter(t *testing.T, dir string, period time.Duration, clock Clock) *Writer {
	t.Helper()

	w, err := New(Config{
		Dir:     dir,
		Period:  period,
		Clock:   clock,
		OnError: func(err error) { t.Errorf("unexpected log rotation error: %v", err) },
	})
	if err != nil {
		t.Fatalf("failed to create the log writer: %v", err)
	}

	return w
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %v, directory holds %v", path, dirEntries(t, filepath.Dir(path)))
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read %v: %v", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func readZip(t *testing.T, path string) map[string]string {
	t.Helper()

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("failed to open the archive %v: %v", path, err)
	}
	defer zr.Close()

	contents := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open the archive entry %v: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read the archive entry %v: %v", f.Name, err)
		}
		contents[f.Name] = string(data)
	}

	return contents
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %v: %v", path, err)
	}

	return string(data)
}

func TestWriterCreatesLogFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node")
	clock := newFakeClock(at(t, "2026-08-26 17:00:00"))

	w := newTestWriter(t, dir, WeeklyPeriod, clock)
	defer w.Close()

	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}

	if got := readFile(t, filepath.Join(dir, DefaultFileName)); got != "hello\n" {
		t.Fatalf("log.txt = %q, want %q", got, "hello\n")
	}
}

// A node started during the day rotates on a midnight boundary, never at the
// time of the day it happened to be started at.
func TestWriterSchedulesTheNextBoundary(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(at(t, "2026-08-26 17:00:00"))

	w := newTestWriter(t, dir, WeeklyPeriod, clock)
	defer w.Close()

	if got, want := w.NextRotation(), at(t, "2026-09-02 00:00:00"); !got.Equal(want) {
		t.Fatalf("NextRotation() = %v, want %v", got, want)
	}
}

func TestWriterRotatesOnTheDefaultPeriodBoundary(t *testing.T) {
	dir := t.TempDir()
	start := at(t, "2026-08-24 00:00:00")
	clock := newFakeClock(start)

	w := newTestWriter(t, dir, WeeklyPeriod, clock)
	defer w.Close()

	if _, err := w.Write([]byte("first period\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}

	clock.waitForWaiters(t, 1)
	clock.AdvanceTo(at(t, "2026-08-31 00:00:00"))

	archive := filepath.Join(dir, "log_2026-08-24_00-00-00_2026-08-31_00-00-00.zip")
	waitForFile(t, archive)

	contents := readZip(t, archive)
	entry := "log_2026-08-24_00-00-00_2026-08-31_00-00-00.txt"
	if got, ok := contents[entry]; !ok || got != "first period\n" {
		t.Fatalf("archive contents = %v, want the entry %v to hold %q", contents, entry, "first period\n")
	}

	// A fresh log.txt is available and the schedule moved on by a week.
	if _, err := w.Write([]byte("second period\n")); err != nil {
		t.Fatalf("failed to write after the rotation: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, DefaultFileName)); got != "second period\n" {
		t.Fatalf("log.txt = %q, want %q", got, "second period\n")
	}
	if got, want := w.NextRotation(), at(t, "2026-09-07 00:00:00"); !got.Equal(want) {
		t.Fatalf("NextRotation() = %v, want %v", got, want)
	}

	// The rotated, uncompressed log file is cleaned up.
	if _, err := os.Stat(filepath.Join(dir, entry)); !os.IsNotExist(err) {
		t.Fatalf("the rotated log file %v was not removed after the compression", entry)
	}
}

func TestWriterRotatesOnACustomPeriod(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(at(t, "2026-08-26 09:00:00"))

	w := newTestWriter(t, dir, 30*time.Minute, clock)
	defer w.Close()

	if got, want := w.NextRotation(), at(t, "2026-08-26 09:30:00"); !got.Equal(want) {
		t.Fatalf("NextRotation() = %v, want %v", got, want)
	}

	if _, err := w.Write([]byte("half hour\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}

	clock.waitForWaiters(t, 1)
	clock.Advance(30 * time.Minute)

	archive := filepath.Join(dir, "log_2026-08-26_09-00-00_2026-08-26_09-30-00.zip")
	waitForFile(t, archive)

	if got, want := w.NextRotation(), at(t, "2026-08-26 10:00:00"); !got.Equal(want) {
		t.Fatalf("NextRotation() = %v, want %v", got, want)
	}
}

// A restart which happens before the next boundary does not rotate the log
// file, and the period start of the active log file survives it.
func TestWriterRestartBeforeTheNextRotation(t *testing.T) {
	tests := []struct {
		name     string
		period   time.Duration
		downtime time.Duration
		wantNext string
	}{
		// A boundary grid which divides a day evenly is unaffected by the
		// restart.
		{"twelve hourly", 12 * time.Hour, 3 * time.Hour, "2026-08-27 00:00:00"},
		// The default period is re-anchored on the midnight of the day the
		// node came back up.
		{"weekly", WeeklyPeriod, 48 * time.Hour, "2026-09-04 00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			clock := newFakeClock(at(t, "2026-08-26 17:00:00"))

			w := newTestWriter(t, dir, tt.period, clock)
			if _, err := w.Write([]byte("before the restart\n")); err != nil {
				t.Fatalf("failed to write to the log file: %v", err)
			}
			periodStart := w.PeriodStart()
			if err := w.Close(); err != nil {
				t.Fatalf("failed to close the log writer: %v", err)
			}

			clock.Advance(tt.downtime)

			restarted := newTestWriter(t, dir, tt.period, clock)
			defer restarted.Close()

			if got := restarted.PeriodStart(); !got.Equal(periodStart) {
				t.Fatalf("PeriodStart() after the restart = %v, want %v", got, periodStart)
			}
			if got, want := restarted.NextRotation(), at(t, tt.wantNext); !got.Equal(want) {
				t.Fatalf("NextRotation() after the restart = %v, want %v", got, want)
			}

			if _, err := restarted.Write([]byte("after the restart\n")); err != nil {
				t.Fatalf("failed to write after the restart: %v", err)
			}

			// Nothing was archived and the existing entries were kept.
			for _, name := range dirEntries(t, dir) {
				if strings.HasSuffix(name, ".zip") {
					t.Fatalf("the restart created the archive %v, want no rotation", name)
				}
			}
			if got, want := readFile(t, filepath.Join(dir, DefaultFileName)), "before the restart\nafter the restart\n"; got != want {
				t.Fatalf("log.txt = %q, want %q", got, want)
			}
		})
	}
}

// Without a state file the period start of an existing log file is inferred as
// the preceding boundary, so the archive name still describes a full period.
func TestWriterInfersThePeriodStartOfAnExistingLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, DefaultFileName), []byte("legacy\n"), 0644); err != nil {
		t.Fatalf("failed to seed the log file: %v", err)
	}

	clock := newFakeClock(at(t, "2026-08-28 09:00:00"))
	w := newTestWriter(t, dir, WeeklyPeriod, clock)
	defer w.Close()

	if got, want := w.PeriodStart(), at(t, "2026-08-28 00:00:00"); !got.Equal(want) {
		t.Fatalf("PeriodStart() = %v, want %v", got, want)
	}
}

func TestWriterRotateArchivesImmediately(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(at(t, "2026-08-26 09:00:00"))

	w := newTestWriter(t, dir, WeeklyPeriod, clock)
	defer w.Close()

	if _, err := w.Write([]byte("manual\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}

	clock.Advance(time.Hour)
	if err := w.Rotate(); err != nil {
		t.Fatalf("failed to rotate: %v", err)
	}

	archive := filepath.Join(dir, "log_2026-08-26_09-00-00_2026-08-26_10-00-00.zip")
	waitForFile(t, archive)

	if got := readFile(t, filepath.Join(dir, DefaultFileName)); got != "" {
		t.Fatalf("the reopened log.txt = %q, want it to be empty", got)
	}
}

func TestWriterRejectsAnInvalidPeriod(t *testing.T) {
	for _, period := range []time.Duration{0, -time.Hour} {
		if _, err := New(Config{Dir: t.TempDir(), Period: period}); err == nil {
			t.Fatalf("New with the period %v succeeded, want an error", period)
		}
	}
}

func TestWriterWriteAfterClose(t *testing.T) {
	w := newTestWriter(t, t.TempDir(), WeeklyPeriod, newFakeClock(at(t, "2026-08-26 09:00:00")))
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close the log writer: %v", err)
	}

	if _, err := w.Write([]byte("late\n")); err == nil {
		t.Fatal("a write on a closed log writer succeeded, want an error")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("the second Close returned an error: %v", err)
	}
}

// Every entry written while the log file rotates ends up either in the active
// log file or in one of the archives, exactly once.
func TestConcurrentWritesDuringRotation(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(at(t, "2026-08-26 09:00:00"))

	w := newTestWriter(t, dir, time.Hour, clock)

	const (
		writers = 8
		entries = 200
	)

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entries; j++ {
				if _, err := w.Write([]byte(fmt.Sprintf("writer-%d-entry-%d\n", id, j))); err != nil {
					t.Errorf("failed to write to the log file: %v", err)
					return
				}
			}
		}(i)
	}

	for i := 0; i < 5; i++ {
		clock.Advance(time.Hour)
		if err := w.Rotate(); err != nil {
			t.Fatalf("failed to rotate: %v", err)
		}
	}

	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close the log writer: %v", err)
	}

	seen := make(map[string]int)
	countLines := func(content string) {
		for _, line := range strings.Split(content, "\n") {
			if line != "" {
				seen[line]++
			}
		}
	}

	countLines(readFile(t, filepath.Join(dir, DefaultFileName)))
	for _, name := range dirEntries(t, dir) {
		if !strings.HasSuffix(name, ".zip") {
			continue
		}
		for _, content := range readZip(t, filepath.Join(dir, name)) {
			countLines(content)
		}
	}

	for i := 0; i < writers; i++ {
		for j := 0; j < entries; j++ {
			line := fmt.Sprintf("writer-%d-entry-%d", i, j)
			if seen[line] != 1 {
				t.Fatalf("the entry %q was found %d times across log.txt and the archives, want exactly 1", line, seen[line])
			}
		}
	}
}

// A failing compression leaves the rotated log file untouched and does not
// leave a partial archive behind.
func TestCompressFailureKeepsTheRotatedLog(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "log_2026-08-24_03-00-00_2026-08-31_03-00-00.txt")
	if err := os.WriteFile(src, []byte("period\n"), 0644); err != nil {
		t.Fatalf("failed to seed the rotated log file: %v", err)
	}

	// The archive directory does not exist, so the temporary file cannot be
	// created.
	zipPath := filepath.Join(dir, "missing", "log.zip")
	if err := compress(src, filepath.Base(src), zipPath); err == nil {
		t.Fatal("compress succeeded, want an error")
	}

	if got := readFile(t, src); got != "period\n" {
		t.Fatalf("the rotated log file = %q, want it to be preserved", got)
	}
	if _, err := os.Stat(zipPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("a partial archive was left behind")
	}
}

// A rotation whose archival fails keeps on logging into a usable log file.
func TestRotationSurvivesAnArchivalFailure(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock(at(t, "2026-08-26 09:00:00"))

	var mu sync.Mutex
	var failures []error
	w, err := New(Config{
		Dir:    dir,
		Period: time.Hour,
		Clock:  clock,
		OnError: func(err error) {
			mu.Lock()
			defer mu.Unlock()
			failures = append(failures, err)
		},
	})
	if err != nil {
		t.Fatalf("failed to create the log writer: %v", err)
	}

	if _, err := w.Write([]byte("before\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}

	// Occupying the name of the archive with a directory makes the rename of
	// the temporary archive fail.
	if err := os.Mkdir(filepath.Join(dir, "log_2026-08-26_09-00-00_2026-08-26_10-00-00.zip"), 0755); err != nil {
		t.Fatalf("failed to block the archive name: %v", err)
	}

	clock.Advance(time.Hour)
	if err := w.Rotate(); err != nil {
		t.Fatalf("failed to rotate: %v", err)
	}

	if _, err := w.Write([]byte("after\n")); err != nil {
		t.Fatalf("failed to write after the failed archival: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close the log writer: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(failures) == 0 {
		t.Fatal("the failed archival was not reported")
	}

	// The rotated log file is kept, so nothing is lost.
	rotated := filepath.Join(dir, "log_2026-08-26_09-00-00_2026-08-26_10-00-00.txt")
	if got := readFile(t, rotated); got != "before\n" {
		t.Fatalf("the rotated log file = %q, want %q", got, "before\n")
	}
	if got := readFile(t, filepath.Join(dir, DefaultFileName)); got != "after\n" {
		t.Fatalf("log.txt = %q, want %q", got, "after\n")
	}
}
