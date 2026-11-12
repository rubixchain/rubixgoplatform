package command

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

func newFileLogger(name string, w io.Writer) logger.Logger {
	return logger.New(&logger.LoggerOptions{
		Name:   name,
		Level:  logger.Debug,
		Color:  []logger.ColorOption{logger.ColorOff}, // as Run sets for the log file
		Output: []io.Writer{w},
	})
}

func writeLogs(t *testing.T, path string, rotation bool, maxSize int, msgs ...string) {
	t.Helper()
	fp, err := openLogFile(path, rotation, maxSize)
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	log := newFileLogger("Main", fp)
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

// Two-process tests: a running node and another CLI command writing to the
// same log file. The CLI command runs in a child process, the test binary
// re-run as TestCLILogHelperProcess.

const (
	helperEnvPath  = "RUBIX_TEST_CLI_LOG_PATH"
	helperEnvFrom  = "RUBIX_TEST_CLI_LOG_FROM"
	helperEnvCount = "RUBIX_TEST_CLI_LOG_COUNT"
	helperEnvDelay = "RUBIX_TEST_CLI_LOG_DELAY"
)

var logPadding = strings.Repeat("p", 100)

// A whole node or CLI test line, as written by newFileLogger.
var testLogLine = regexp.MustCompile(`^\S+ \[INFO\]  (?:Main: node-\d+-\d+|CLI: cli-\d+) p{100}$`)

// TestCLILogHelperProcess is the CLI command of the two-process tests. It
// writes lines cli-<from> .. cli-<from+count-1> through openCLILogFile, as
// Run does for commands other than run.
func TestCLILogHelperProcess(t *testing.T) {
	path := os.Getenv(helperEnvPath)
	if path == "" {
		t.Skip("child process of the two-process log file tests")
	}
	from, _ := strconv.Atoi(os.Getenv(helperEnvFrom))
	count, _ := strconv.Atoi(os.Getenv(helperEnvCount))
	delay, _ := time.ParseDuration(os.Getenv(helperEnvDelay))

	fp, err := openCLILogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	log := newFileLogger("CLI", fp)
	for i := from; i < from+count; i++ {
		log.Info(fmt.Sprintf("cli-%d %s", i, logPadding))
		time.Sleep(delay)
	}
}

// cliProcess returns a child process writing CLI lines cli-<from>.. to path.
func cliProcess(path string, from, count int, delay time.Duration) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLILogHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		helperEnvPath+"="+path,
		helperEnvFrom+"="+strconv.Itoa(from),
		helperEnvCount+"="+strconv.Itoa(count),
		helperEnvDelay+"="+delay.String(),
	)
	return cmd
}

func runCLIProcess(t *testing.T, path string, from, count int) {
	t.Helper()
	if out, err := cliProcess(path, from, count, 0).CombinedOutput(); err != nil {
		t.Fatalf("CLI process: %v\n%s", err, out)
	}
}

// readAllLogs waits until every rotated file is compressed and returns the
// lines of the backups, oldest first, followed by those of log.txt.
func readAllLogs(t *testing.T, path string) (lines []string, backups int) {
	t.Helper()
	dir := filepath.Dir(path)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if pending, _ := filepath.Glob(filepath.Join(dir, "log-*.txt")); len(pending) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rotated log files not compressed")
		}
	}
	gzs, _ := filepath.Glob(filepath.Join(dir, "log-*.txt.gz")) // sorted, and names sort by time
	var all strings.Builder
	for _, name := range gzs {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := io.Copy(&all, zr); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f.Close()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	all.Write(data)
	return strings.Split(strings.TrimSuffix(all.String(), "\n"), "\n"), len(gzs)
}

// countLines counts the lines containing "Main: <id> " or "CLI: <id> " for
// each id, and returns the lines that are not whole test lines.
func countLines(lines []string) (counts map[string]int, broken []string) {
	counts = map[string]int{}
	for _, l := range lines {
		if !testLogLine.MatchString(l) {
			broken = append(broken, l)
			continue
		}
		_, msg, _ := strings.Cut(l, ": ")
		id, _, _ := strings.Cut(msg, " ")
		counts[id]++
	}
	return counts, broken
}

// After the node has rotated its log, lines appended by another command must
// not be overwritten by the node's later writes.
func TestCLILinesKeptAfterNodeRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.txt")
	fp, err := openLogFile(path, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	node := newFileLogger("Main", fp)

	// Past 1 MiB, so the node rotates once.
	n := 0
	for ; n < 8000; n++ {
		node.Info(fmt.Sprintf("node-0-%d %s", n, logPadding))
	}
	// The node and a CLI command take turns writing to the new log.txt.
	for round := 0; round < 3; round++ {
		runCLIProcess(t, path, round*50, 50)
		for end := n + 50; n < end; n++ {
			node.Info(fmt.Sprintf("node-0-%d %s", n, logPadding))
		}
	}
	fp.Close()

	lines, backups := readAllLogs(t, path)
	if backups != 1 {
		t.Fatalf("got %d rotations, want 1", backups)
	}
	counts, broken := countLines(lines)
	if len(broken) > 0 {
		t.Fatalf("%d broken lines, first %.120q", len(broken), broken[0])
	}
	for i := 0; i < n; i++ {
		if c := counts[fmt.Sprintf("node-0-%d", i)]; c != 1 {
			t.Fatalf("node line %d found %d times", i, c)
		}
	}
	for i := 0; i < 150; i++ {
		if c := counts[fmt.Sprintf("cli-%d", i)]; c != 1 {
			t.Fatalf("CLI line %d found %d times", i, c)
		}
	}
	// log.txt ends with the turns, in order.
	var want []string
	for round := 0; round < 3; round++ {
		for i := round * 50; i < round*50+50; i++ {
			want = append(want, fmt.Sprintf("CLI: cli-%d ", i))
		}
		for i := 8000 + round*50; i < 8000+round*50+50; i++ {
			want = append(want, fmt.Sprintf("Main: node-0-%d ", i))
		}
	}
	active := strings.Split(strings.TrimSuffix(readFile(t, path), "\n"), "\n")
	if len(active) < len(want) {
		t.Fatalf("log.txt has %d lines, want at least %d", len(active), len(want))
	}
	for i, line := range active[len(active)-len(want):] {
		if !strings.Contains(line, want[i]) {
			t.Fatalf("log.txt line %q, want %q", line, want[i])
		}
	}
}

// The node and CLI commands writing at the same time while the node rotates
// its log: no node line is lost, duplicated or broken.
//
// A CLI line written while lumberjack is replacing log.txt can still be lost,
// since lumberjack truncates the new file; that window is a few system calls
// per rotation, so at most 1% of the CLI lines may be missing or broken.
func TestCLIAndNodeWriteConcurrently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.txt")
	fp, err := openLogFile(path, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	node := newFileLogger("Main", fp)

	const cliProcs, cliLines = 2, 300
	var cliDone atomic.Bool
	var cliErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cliDone.Store(true)
		var procs []*exec.Cmd
		for p := 0; p < cliProcs; p++ {
			cmd := cliProcess(path, p*cliLines, cliLines, 200*time.Microsecond)
			if err := cmd.Start(); err != nil {
				cliErr = err
				return
			}
			procs = append(procs, cmd)
		}
		for _, cmd := range procs {
			if err := cmd.Wait(); err != nil && cliErr == nil {
				cliErr = err
			}
		}
	}()

	// Node goroutines write at least 2.5 MiB, and keep writing until the CLI
	// commands are done.
	const goroutines, minLines = 4, 4000
	written := make([]int, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			i := 0
			for ; i < minLines || !cliDone.Load(); i++ {
				node.Info(fmt.Sprintf("node-%d-%d %s", g, i, logPadding))
			}
			written[g] = i
		}(g)
	}
	wg.Wait()
	fp.Close()
	if cliErr != nil {
		t.Fatalf("CLI process: %v", cliErr)
	}

	lines, backups := readAllLogs(t, path)
	if backups < 2 {
		t.Fatalf("got %d rotations, want at least 2", backups)
	}
	counts, broken := countLines(lines)
	for g, n := range written {
		for i := 0; i < n; i++ {
			if c := counts[fmt.Sprintf("node-%d-%d", g, i)]; c != 1 {
				t.Fatalf("node line %d-%d found %d times", g, i, c)
			}
		}
	}
	missing := 0
	for i := 0; i < cliProcs*cliLines; i++ {
		switch c := counts[fmt.Sprintf("cli-%d", i)]; c {
		case 0:
			missing++
		case 1:
		default:
			t.Fatalf("CLI line %d found %d times", i, c)
		}
	}
	if missing > 0 || len(broken) > 0 {
		t.Logf("%d of %d CLI lines missing, %d broken lines", missing, cliProcs*cliLines, len(broken))
	}
	if limit := cliProcs * cliLines / 100; missing > limit || len(broken) > limit {
		t.Fatalf("%d CLI lines missing and %d broken, want at most %d each; first broken: %.120q",
			missing, len(broken), limit, append(broken, "")[0])
	}
}

// A CLI command finding no log file waits for the node to create the new one
// instead of creating a file the node would truncate; with no node it creates
// the file itself.
func TestCLILogFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	w, err := openCLILogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// The node creates the new log file shortly after.
	created := make(chan struct{})
	go func() {
		defer close(created)
		time.Sleep(5 * time.Millisecond)
		os.WriteFile(path, []byte("from the node\n"), 0644)
	}()
	if _, err := io.WriteString(w, "from the CLI\n"); err != nil {
		t.Fatal(err)
	}
	<-created
	if got := readFile(t, path); got != "from the node\nfrom the CLI\n" {
		t.Fatalf("log.txt = %q", got)
	}

	// No node: the file is created, 0644 less the umask like openLogFile.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "only the CLI\n"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "only the CLI\n" {
		t.Fatalf("log.txt = %q", got)
	}
	plain := filepath.Join(dir, "plain.txt")
	writeLogs(t, plain, false, 1)
	if got, want := fileMode(t, path), fileMode(t, plain); got != want {
		t.Fatalf("log.txt has mode %v, want %v", got, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
