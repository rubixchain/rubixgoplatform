package logger

import "gopkg.in/natefinch/lumberjack.v2"

// Size-based rotation of the node log file, done entirely by lumberjack: when
// a write would take the file past MaxSize, it is renamed to
// <name>-<2006-01-02T15-04-05.000><ext>, gzip-compressed in the background and
// replaced by a fresh file. Backups are never deleted.

// RotatingFile is a log file managed by lumberjack. It is safe for concurrent
// writes and can be used as a LoggerOptions output.
type RotatingFile = lumberjack.Logger

// NewRotatingFile opens path for appending and rotates it once it reaches
// maxSize MiB.
func NewRotatingFile(path string, maxSize int) *RotatingFile {
	return &RotatingFile{
		Filename:   path,
		MaxSize:    maxSize, // MiB
		MaxBackups: 0,       // keep every backup
		MaxAge:     0,       // never delete backups by age
		// Log lines are stamped with time.Now() in local time, so backup names
		// use local time too.
		LocalTime: true,
		Compress:  true,
	}
}
