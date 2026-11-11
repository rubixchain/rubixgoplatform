package config

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/rubixchain/rubixgoplatform/types"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(path.Join(dir, "config.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write the test config.toml: %v", err)
	}

	return dir
}

// coreConfig builds a config.toml whose [core] table carries the given extra
// settings.
func coreConfig(settings string) string {
	return `
[core]
node_index = 0
network_mode = "localnet"
` + settings + `
[db]
host = "localhost"
username = "rubix"
password = "rubixpass"
db_name = "rubix"
`
}

var baseConfig = coreConfig("")

func TestParseConfigLogRotation(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantOn     bool
		wantPeriod time.Duration
	}{
		{
			// An existing deployment which never heard about the rotation.
			name:       "missing fields fall back to the defaults",
			config:     baseConfig,
			wantOn:     false,
			wantPeriod: 7 * 24 * time.Hour,
		},
		{
			name:       "rotation disabled explicitly",
			config:     coreConfig("log_rotation = false\nlog_rotation_period = \"24h\"\n"),
			wantOn:     false,
			wantPeriod: 24 * time.Hour,
		},
		{
			name:       "rotation enabled with the default period",
			config:     coreConfig("log_rotation = true\n"),
			wantOn:     true,
			wantPeriod: 7 * 24 * time.Hour,
		},
		{
			name:       "weekly rotation",
			config:     coreConfig("log_rotation = true\nlog_rotation_period = \"7d\"\n"),
			wantOn:     true,
			wantPeriod: 7 * 24 * time.Hour,
		},
		{
			name:       "daily rotation",
			config:     coreConfig("log_rotation = true\nlog_rotation_period = \"24h\"\n"),
			wantOn:     true,
			wantPeriod: 24 * time.Hour,
		},
		{
			name:       "half hourly rotation",
			config:     coreConfig("log_rotation = true\nlog_rotation_period = \"30m\"\n"),
			wantOn:     true,
			wantPeriod: 30 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userConfig, err := ParseConfigFromPath(writeConfig(t, tt.config))
			if err != nil {
				t.Fatalf("ParseConfigFromPath returned an unexpected error: %v", err)
			}

			logConfig, err := ResolveLogConfig(userConfig)
			if err != nil {
				t.Fatalf("ResolveLogConfig returned an unexpected error: %v", err)
			}
			if logConfig.Rotation != tt.wantOn {
				t.Fatalf("Rotation = %v, want %v", logConfig.Rotation, tt.wantOn)
			}
			if logConfig.RotationPeriod != tt.wantPeriod {
				t.Fatalf("RotationPeriod = %v, want %v", logConfig.RotationPeriod, tt.wantPeriod)
			}

			rubixConfig, err := CreateRubixConfigFromUserConfig(userConfig, t.TempDir())
			if err != nil {
				t.Fatalf("CreateRubixConfigFromUserConfig returned an unexpected error: %v", err)
			}
			if rubixConfig.LogConfig != logConfig {
				t.Fatalf("RubixConfig.LogConfig = %+v, want %+v", rubixConfig.LogConfig, logConfig)
			}
		})
	}
}

func TestParseConfigInvalidLogRotationPeriod(t *testing.T) {
	for _, period := range []string{"xyz", "0", "-7d"} {
		t.Run(period, func(t *testing.T) {
			config := coreConfig("log_rotation = true\nlog_rotation_period = \"" + period + "\"\n")

			if _, err := ParseConfigFromPath(writeConfig(t, config)); err == nil {
				t.Fatalf("ParseConfigFromPath accepted the period %q, want a configuration error", period)
			}

			userConfig := types.UserConfig{
				Core: types.CoreConfig{LogRotation: true, LogRotationPeriod: period},
			}
			if _, err := ResolveLogConfig(userConfig); err == nil {
				t.Fatalf("ResolveLogConfig accepted the period %q, want a configuration error", period)
			}
		})
	}
}

// The generated template is a valid config.toml which parses into the
// documented defaults.
func TestConfigTemplateDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := CreateConfigFileFromTemplate(dir); err != nil {
		t.Fatalf("failed to create the config file from the template: %v", err)
	}

	userConfig, err := ParseConfigFromPath(dir)
	if err != nil {
		t.Fatalf("failed to parse the generated config file: %v", err)
	}

	if userConfig.Core.LogRotation {
		t.Fatal("the template enables the log rotation, want it disabled by default")
	}
	if userConfig.Core.LogRotationPeriod != "7d" {
		t.Fatalf("the template log_rotation_period = %q, want %q", userConfig.Core.LogRotationPeriod, "7d")
	}
}
