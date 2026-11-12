package command

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// The running node and other CLI commands write to the same log file. Both
// have to append (O_APPEND) so neither overwrites the other's lines:
//
//   - The node rotates the file with lumberjack. After a rotation lumberjack
//     opens the new file without O_APPEND, so appendingRotatingFile closes it
//     right after the rotation; lumberjack's next write reopens the file with
//     O_APPEND.
//   - Other commands open the file for each write (cliLogFile), so a command
//     that runs across a rotation does not keep writing to the rotated file.
//
// A line written by another command while lumberjack is replacing the file can
// still be lost, as lumberjack truncates the new file; cliLogFile keeps that
// window small by not creating a missing file straight away.

// openLogFile opens the node's log file for appending. With rotation on, the
// file is rotated once it reaches maxSize MiB.
func openLogFile(path string, rotation bool, maxSize int) (io.WriteCloser, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil || !rotation {
		return f, err
	}
	// lumberjack would create a missing file as 0600; it keeps the mode of the
	// existing file across rotations, so creating it here keeps logs at 0644.
	info, err := f.Stat()
	f.Close()
	if err != nil {
		return nil, err
	}
	return &appendingRotatingFile{
		file: logger.NewRotatingFile(path, maxSize),
		path: path,
		info: info,
	}, nil
}

// appendingRotatingFile is a rotating log file that is always written with
// O_APPEND, so lines appended by other processes are not overwritten.
type appendingRotatingFile struct {
	mu   sync.Mutex
	file *logger.RotatingFile
	path string
	info os.FileInfo // the log file being written to
}

func (f *appendingRotatingFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.file.Write(p)
	// A different (or missing) file at path means this write rotated it, or
	// it was removed. Closing makes the next write reopen it with O_APPEND.
	info, statErr := os.Stat(f.path)
	if statErr != nil || !os.SameFile(f.info, info) {
		f.file.Close()
		f.info = info
	}
	return n, err
}

func (f *appendingRotatingFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Close()
}

const (
	// How long cliLogFile waits for a missing log file to reappear before
	// creating it.
	cliLogOpenRetries    = 20
	cliLogOpenRetryDelay = time.Millisecond
)

// openCLILogFile returns the log file of a command other than run. It never
// rotates the file, which may be rotated by a running node.
func openCLILogFile(path string) (io.WriteCloser, error) {
	// Fail early, as before, if the log file cannot be written.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	f.Close()
	return cliLogFile{path: path}, nil
}

// cliLogFile opens the log file for each write and appends to it.
type cliLogFile struct {
	path string
}

func (c cliLogFile) Write(p []byte) (int, error) {
	f, err := c.open()
	if err != nil {
		return 0, err
	}
	n, err := f.Write(p)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return n, err
}

// open opens the log file for appending. A missing file usually means a node
// is rotating it, and lumberjack would truncate a file created now, so it
// waits briefly for the new file before creating one.
func (c cliLogFile) open() (*os.File, error) {
	for i := 0; i < cliLogOpenRetries; i++ {
		f, err := os.OpenFile(c.path, os.O_APPEND|os.O_WRONLY, 0)
		if !os.IsNotExist(err) {
			return f, err
		}
		time.Sleep(cliLogOpenRetryDelay)
	}
	return os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func (cliLogFile) Close() error { return nil }
