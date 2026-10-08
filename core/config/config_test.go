package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, core string) string {
	t.Helper()
	dir := t.TempDir()
	data := "[core]\nnode_index = 0\nnetwork_mode = \"localnet\"\n" + core
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLogRotationConfig(t *testing.T) {
	tests := []struct {
		name         string
		core         string
		wantRotation bool
		wantDuration string
	}{
		{"defaults for old config", "", true, "7d"},
		{"custom duration", "log_rotation_duration = \"12h\"\n", true, "12h"},
		{"disabled", "log_rotation = false\n", false, "7d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseConfigFromPath(writeConfig(t, tc.core))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Core.LogRotation != tc.wantRotation || cfg.Core.LogRotationDuration != tc.wantDuration {
				t.Fatalf("got log_rotation=%v log_rotation_duration=%q, want %v %q",
					cfg.Core.LogRotation, cfg.Core.LogRotationDuration, tc.wantRotation, tc.wantDuration)
			}
		})
	}

	if _, err := ParseConfigFromPath(writeConfig(t, "log_rotation_duration = \"weekly\"\n")); err == nil {
		t.Fatal("expected an error for an invalid log_rotation_duration")
	}
}

func TestParseLogRotationDuration(t *testing.T) {
	valid := map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"1d":  24 * time.Hour,
		"12h": 12 * time.Hour,
		"90s": 90 * time.Second,
	}
	for in, want := range valid {
		if got, err := ParseLogRotationDuration(in); err != nil || got != want {
			t.Errorf("ParseLogRotationDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0d", "-1h", "xd", "week"} {
		if _, err := ParseLogRotationDuration(in); err == nil {
			t.Errorf("ParseLogRotationDuration(%q) should fail", in)
		}
	}
}
