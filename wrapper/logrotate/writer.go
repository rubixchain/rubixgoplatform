// Package logrotate provides a concurrency safe io.Writer for the node log
// file which archives the log into a timestamped ZIP file on a configurable
// schedule, without requiring a node restart.
package logrotate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultFileName is the name of the active node log file.
	DefaultFileName = "log.txt"

	// stateFileName holds the start timestamp of the active log file so that
	// the period covered by an archive stays accurate across node restarts.
	stateFileName = ".log_rotation_state"

	// timestampLayout is a filesystem safe timestamp used in archive names.
	timestampLayout = "2006-01-02_15-04-05"

	logFilePerm = 0644
)

// Clock abstracts the time source so that the scheduler can be driven
// deterministically from tests without waiting for a rotation period.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Config holds the settings of a rotating log writer.
type Config struct {
	// Dir is the directory holding the active log file. The archives are
	// written to the same directory.
	Dir string

	// FileName is the name of the active log file. Defaults to log.txt.
	FileName string

	// Period is the rotation interval. It must be greater than zero.
	Period time.Duration

	// Clock is the time source. Defaults to the system clock.
	Clock Clock

	// OnError, when set, is called with the errors which are hit by the
	// background rotation and compression, since those have no caller to
	// report to. It must not write to this writer.
	OnError func(error)
}

// Writer is an io.Writer over the active log file which rotates and archives
// it on the configured schedule. It is safe for concurrent use.
type Writer struct {
	mu           sync.Mutex
	dir          string
	name         string
	base         string
	ext          string
	period       time.Duration
	clock        Clock
	onError      func(error)
	f            *os.File
	periodStart  time.Time
	nextRotation time.Time
	closed       bool
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// New opens the active log file and starts the rotation scheduler.
//
// The start of the period covered by an already existing log file is restored
// from the state file written by the previous run, so a restart never rotates
// immediately and the archive keeps naming the period it actually covers. When
// the state is missing, the start is inferred as the rotation boundary
// preceding the current time.
func New(cfg Config) (*Writer, error) {
	if cfg.Period <= 0 {
		return nil, fmt.Errorf("log rotation period must be greater than zero, got %v", cfg.Period)
	}
	if cfg.Dir == "" {
		cfg.Dir = "."
	}
	if cfg.FileName == "" {
		cfg.FileName = DefaultFileName
	}
	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}

	if err := os.MkdirAll(cfg.Dir, os.ModeDir|os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create log directory %v, err: %v", cfg.Dir, err)
	}

	ext := filepath.Ext(cfg.FileName)
	w := &Writer{
		dir:     cfg.Dir,
		name:    cfg.FileName,
		base:    strings.TrimSuffix(cfg.FileName, ext),
		ext:     ext,
		period:  cfg.Period,
		clock:   cfg.Clock,
		onError: cfg.OnError,
		stopCh:  make(chan struct{}),
	}

	now := w.clock.Now()
	w.periodStart = w.resolvePeriodStart(now)
	w.nextRotation = NextBoundary(now, w.period)

	f, err := os.OpenFile(w.activePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %v, err: %v", w.activePath(), err)
	}
	w.f = f

	w.saveState()

	w.wg.Add(1)
	go w.run()

	return w, nil
}

// Write appends to the active log file, rotating it first when the rotation
// boundary has been crossed. Writes and rotations are mutually exclusive, so
// no entry is lost or interleaved with a rotation.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, errors.New("log writer is closed")
	}

	if !w.nextRotation.After(w.clock.Now()) {
		if err := w.rotateLocked(w.nextRotation); err != nil {
			w.reportError(err)
		}
	}

	return w.f.Write(p)
}

// Rotate archives the active log file right away, using the current time as
// the end of the period, and re-arms the scheduler on the regular boundaries.
func (w *Writer) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return errors.New("log writer is closed")
	}

	return w.rotateLocked(w.clock.Now())
}

// Close flushes and closes the active log file and stops the scheduler. It
// waits for any archive which is still being compressed.
func (w *Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	f := w.f
	w.f = nil
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()

	if f == nil {
		return nil
	}
	if err := f.Sync(); err != nil {
		w.reportError(fmt.Errorf("failed to flush log file, err: %v", err))
	}

	return f.Close()
}

// FilePath returns the path of the active log file.
func (w *Writer) FilePath() string {
	return w.activePath()
}

// NextRotation returns the instant at which the next rotation is scheduled.
func (w *Writer) NextRotation() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nextRotation
}

// PeriodStart returns the start of the period covered by the active log file.
func (w *Writer) PeriodStart() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.periodStart
}

// run waits for the rotation boundary and rotates even when the node is idle,
// so that an archive always covers the period named by its filename.
func (w *Writer) run() {
	defer w.wg.Done()

	for {
		wait := w.NextRotation().Sub(w.clock.Now())
		if wait < 0 {
			wait = 0
		}

		select {
		case <-w.stopCh:
			return
		case <-w.clock.After(wait):
			w.rotateIfDue()
		}
	}
}

func (w *Writer) rotateIfDue() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.nextRotation.After(w.clock.Now()) {
		return
	}

	if err := w.rotateLocked(w.nextRotation); err != nil {
		w.reportError(err)
	}
}

// rotateLocked archives the active log file for the period ending at
// periodEnd and reopens a fresh one. It must be called with the lock held.
//
// The swap itself is a rename followed by a fresh open, so logging resumes
// immediately; the comparatively slow compression runs in the background on
// the renamed file, which no longer receives writes.
func (w *Writer) rotateLocked(periodEnd time.Time) error {
	start := w.periodStart
	now := w.clock.Now()

	// Re-arm the schedule first, so that a failing rotation is not retried on
	// every single write.
	w.periodStart = periodEnd
	w.nextRotation = NextBoundary(now, w.period)
	w.saveState()

	if w.f != nil {
		if err := w.f.Sync(); err != nil {
			w.reportError(fmt.Errorf("failed to flush log file before rotation, err: %v", err))
		}
		if err := w.f.Close(); err != nil {
			w.reportError(fmt.Errorf("failed to close log file before rotation, err: %v", err))
		}
		w.f = nil
	}

	active := w.activePath()
	rotatedName := w.archiveName(start, periodEnd) + w.ext
	rotatedPath := filepath.Join(w.dir, rotatedName)

	renameErr := os.Rename(active, rotatedPath)

	// Reopen unconditionally: when the rename failed the active file still
	// holds the entries of the period, and appending to it loses nothing.
	f, err := os.OpenFile(active, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return fmt.Errorf("failed to reopen log file %v after rotation, err: %v", active, err)
	}
	w.f = f

	if renameErr != nil {
		return fmt.Errorf("failed to archive log file %v, err: %v", active, renameErr)
	}

	zipPath := filepath.Join(w.dir, w.archiveName(start, periodEnd)+".zip")

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := compress(rotatedPath, rotatedName, zipPath); err != nil {
			// The rotated log file is deliberately left in place, so a failed
			// compression never destroys the log.
			w.reportError(err)
			return
		}
		if err := os.Remove(rotatedPath); err != nil {
			w.reportError(fmt.Errorf("failed to remove archived log file %v, err: %v", rotatedPath, err))
		}
	}()

	return nil
}

// archiveName builds the `log_<START>_<END>` base name of an archive, e.g.
// log_2026-08-24_03-00-00_2026-08-31_03-00-00.
func (w *Writer) archiveName(start, end time.Time) string {
	return fmt.Sprintf("%s_%s_%s", w.base, start.Format(timestampLayout), end.Format(timestampLayout))
}

func (w *Writer) activePath() string {
	return filepath.Join(w.dir, w.name)
}

func (w *Writer) statePath() string {
	return filepath.Join(w.dir, stateFileName)
}

// resolvePeriodStart determines the start of the period covered by the active
// log file: the timestamp persisted by the previous run when it is usable,
// the preceding rotation boundary when an unaccounted log file is found, and
// the current time when the log file is created from scratch.
func (w *Writer) resolvePeriodStart(now time.Time) time.Time {
	if _, err := os.Stat(w.activePath()); err != nil {
		return now
	}

	if start, ok := w.loadState(); ok && start.Before(now) {
		return start
	}

	return PreviousBoundary(now, w.period)
}

func (w *Writer) loadState() (time.Time, bool) {
	data, err := os.ReadFile(w.statePath())
	if err != nil {
		return time.Time{}, false
	}

	start, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false
	}

	return start.Local(), true
}

func (w *Writer) saveState() {
	data := []byte(w.periodStart.Format(time.RFC3339Nano))
	if err := os.WriteFile(w.statePath(), data, logFilePerm); err != nil {
		w.reportError(fmt.Errorf("failed to persist log rotation state, err: %v", err))
	}
}

func (w *Writer) reportError(err error) {
	if err == nil {
		return
	}
	if w.onError != nil {
		w.onError(err)
		return
	}
	fmt.Fprintf(os.Stderr, "log rotation: %v\n", err)
}

var _ io.Writer = (*Writer)(nil)
