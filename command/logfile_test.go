package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rubixchain/rubixgoplatform/wrapper/logrotate"
)

func newLogTestCommand(t *testing.T, config string) *Command {
	t.Helper()

	dir := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0644); err != nil {
			t.Fatalf("failed to write the test config.toml: %v", err)
		}
	}

	return &Command{
		nodeConfigPath: dir,
		logFile:        filepath.Join(dir, logrotate.DefaultFileName),
	}
}

// logTestConfig builds a config.toml whose [core] table carries the given
// extra settings.
func logTestConfig(settings string) string {
	return `
[core]
node_index = 0
network_mode = "localnet"
` + settings
}

// Without the rotation the log file is the plain, appended log.txt.
func TestOpenLogFileWithoutRotation(t *testing.T) {
	for _, config := range []string{"", logTestConfig(""), logTestConfig("log_rotation = false\n")} {
		cmd := newLogTestCommand(t, config)

		w, err := cmd.openLogFile()
		if err != nil {
			t.Fatalf("openLogFile returned an unexpected error: %v", err)
		}
		defer cmd.closeLogFile()

		if _, ok := w.(*os.File); !ok {
			t.Fatalf("openLogFile returned %T, want the plain log file", w)
		}
		if cmd.logRotator != nil {
			t.Fatal("openLogFile started a rotator while the rotation is disabled")
		}
		if _, err := os.Stat(cmd.logFile); err != nil {
			t.Fatalf("log.txt was not created: %v", err)
		}
	}
}

func TestOpenLogFileWithRotation(t *testing.T) {
	cmd := newLogTestCommand(t, logTestConfig("log_rotation = true\nlog_rotation_period = \"24h\"\n"))

	w, err := cmd.openLogFile()
	if err != nil {
		t.Fatalf("openLogFile returned an unexpected error: %v", err)
	}
	defer cmd.closeLogFile()

	rotator, ok := w.(*logrotate.Writer)
	if !ok {
		t.Fatalf("openLogFile returned %T, want a rotating writer", w)
	}
	if cmd.logRotator != rotator {
		t.Fatal("openLogFile did not keep the rotator for the shutdown")
	}
	if got, want := rotator.FilePath(), cmd.logFile; got != want {
		t.Fatalf("the rotator writes to %v, want %v", got, want)
	}

	if _, err := w.Write([]byte("entry\n")); err != nil {
		t.Fatalf("failed to write to the log file: %v", err)
	}
	cmd.closeLogFile()

	data, err := os.ReadFile(cmd.logFile)
	if err != nil {
		t.Fatalf("failed to read log.txt: %v", err)
	}
	if string(data) != "entry\n" {
		t.Fatalf("log.txt = %q, want %q", data, "entry\n")
	}
}

func TestOpenLogFileInvalidRotationPeriod(t *testing.T) {
	cmd := newLogTestCommand(t, logTestConfig("log_rotation = true\nlog_rotation_period = \"xyz\"\n"))

	if _, err := cmd.openLogFile(); err == nil {
		cmd.closeLogFile()
		t.Fatal("openLogFile accepted an invalid log_rotation_period, want a configuration error")
	}
}
