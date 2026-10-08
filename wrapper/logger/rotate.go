package logger

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Time-based rotation of the node log file. lumberjack owns the file, the
// rotation itself and the gzip compression; this file only decides when to
// call Rotate() and gives the compressed archives a <name>_<start>_<end>.gz name.

const (
	// lumberjackTimeFormat is the timestamp lumberjack puts in backup names.
	lumberjackTimeFormat = "2006-01-02T15-04-05.000"
	archiveDateFormat    = "2006-01-02"
	// archivePollInterval is how often finished compressions are checked for
	// while lumberjack is still compressing a rotated file.
	archivePollInterval = time.Second
	rotateRetryInterval = time.Minute
)

// RotatingFile is a log file managed by lumberjack. It is safe for concurrent
// writes and can be used as a LoggerOptions output.
type RotatingFile = lumberjack.Logger

// NewRotatingFile opens path for appending. Rotation only happens when
// StartRotation is running; archived files are gzip-compressed.
func NewRotatingFile(path string) *RotatingFile {
	return &RotatingFile{
		Filename:  path,
		MaxSize:   1 << 30, // in MB; effectively disables size-based rotation
		LocalTime: true,    // backup timestamps in local time, like the log lines
		Compress:  true,
	}
}

// StartRotation rotates lj every period. The period is measured from the
// first entry of the current log file, so a node restart continues the running
// period instead of starting a new one. The returned func stops rotation and
// waits for the goroutine to exit.
func StartRotation(lj *RotatingFile, period time.Duration, log Logger) (stop func()) {
	quit := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		start := time.Now()
		if f, err := os.Open(lj.Filename); err == nil {
			if t, ok := firstLogTime(f); ok {
				start = t
			}
			f.Close()
		}

		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-quit:
				return
			case <-timer.C:
			}

			if !time.Now().Before(start.Add(period)) {
				if err := lj.Rotate(); err != nil {
					log.Error("Failed to rotate log file", "err", err)
					timer.Reset(rotateRetryInterval)
					continue
				}
				start = time.Now()
				// First line of the new file; marks the start of the period.
				log.Info("Log file rotated")
			}

			wait := time.Until(start.Add(period))
			if archiveRotatedLogs(lj.Filename, log) {
				wait = min(wait, archivePollInterval)
			}
			timer.Reset(wait)
		}
	}()

	return func() {
		close(quit)
		<-done
	}
}

// archiveRotatedLogs renames lumberjack's compressed backups
// (log-<rotation time>.txt.gz) to log_<start date>_<end date>.gz. A backup is
// renamed only after lumberjack has removed the uncompressed file, which it
// does once the .gz is completely written. It reports whether any backup is
// still waiting to be compressed.
func archiveRotatedLogs(path string, log Logger) (pending bool) {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	prefix := strings.TrimSuffix(filepath.Base(path), ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Error("Failed to list rotated log files", "err", err)
		return false
	}
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), prefix+"-")
		if !ok {
			continue
		}
		ts, compressed := strings.CutSuffix(rest, ".gz")
		end, err := time.ParseInLocation(lumberjackTimeFormat, strings.TrimSuffix(ts, ext), time.Local)
		if err != nil {
			continue // not a lumberjack backup
		}
		gz := filepath.Join(dir, e.Name())
		if _, err := os.Stat(strings.TrimSuffix(gz, ".gz")); !compressed || err == nil {
			pending = true // lumberjack has not finished compressing it yet
			continue
		}

		start := end
		if f, err := os.Open(gz); err == nil {
			if zr, err := gzip.NewReader(f); err == nil {
				if t, ok := firstLogTime(zr); ok {
					start = t
				}
			}
			f.Close()
		}

		name := fmt.Sprintf("%s_%s_%s", prefix, start.Format(archiveDateFormat), end.Format(archiveDateFormat))
		dst := filepath.Join(dir, name+".gz")
		for i := 1; ; i++ {
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				break
			}
			dst = filepath.Join(dir, fmt.Sprintf("%s_%d.gz", name, i))
		}
		if err := os.Rename(gz, dst); err != nil {
			log.Error("Failed to rename rotated log file", "err", err)
		}
	}
	return pending
}

// firstLogTime returns the timestamp of the first log line in r.
func firstLogTime(r io.Reader) (time.Time, bool) {
	field, _ := bufio.NewReader(io.LimitReader(r, 64)).ReadString(' ')
	t, err := time.Parse(TimeFormat, strings.TrimSpace(field))
	return t, err == nil
}
