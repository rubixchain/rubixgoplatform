package config

import (
	"os"
	"path/filepath"
	"testing"
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
		wantMaxSize  int
	}{
		{"defaults for old config", "", true, 30},
		{"disabled", "log_rotation = false\n", false, 30},
		{"custom max size", "log_rotation_max_size = 100\n", true, 100},
		{"disabled with custom max size", "log_rotation = false\nlog_rotation_max_size = 5\n", false, 5},
		{"disabled with zero max size", "log_rotation = false\nlog_rotation_max_size = 0\n", false, 0},
		{"disabled with negative max size", "log_rotation = false\nlog_rotation_max_size = -1\n", false, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseConfigFromPath(writeConfig(t, tc.core))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Core.LogRotation != tc.wantRotation || cfg.Core.LogRotationMaxSize != tc.wantMaxSize {
				t.Fatalf("got log_rotation=%v log_rotation_max_size=%d, want %v %d",
					cfg.Core.LogRotation, cfg.Core.LogRotationMaxSize, tc.wantRotation, tc.wantMaxSize)
			}
		})
	}

	for _, core := range []string{
		"log_rotation_max_size = 0\n",
		"log_rotation_max_size = -1\n",
		"log_rotation_max_size = \"30MB\"\n",
	} {
		if _, err := ParseConfigFromPath(writeConfig(t, core)); err == nil {
			t.Errorf("expected an error for %q", core)
		}
	}
}

func TestConfigTemplateEnablesLogRotation(t *testing.T) {
	dir := t.TempDir()
	if err := CreateConfigFileFromTemplate(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfigFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Core.LogRotation || cfg.Core.LogRotationMaxSize != 30 {
		t.Fatalf("template has log_rotation=%v log_rotation_max_size=%d, want true 30",
			cfg.Core.LogRotation, cfg.Core.LogRotationMaxSize)
	}
}
