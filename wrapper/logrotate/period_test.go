package logrotate

import (
	"testing"
	"time"
)

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		name   string
		period string
		want   time.Duration
	}{
		{"empty falls back to the default", "", 7 * 24 * time.Hour},
		{"days", "7d", 7 * 24 * time.Hour},
		{"single day", "1d", 24 * time.Hour},
		{"hours", "24h", 24 * time.Hour},
		{"half a day", "12h", 12 * time.Hour},
		{"minutes", "30m", 30 * time.Minute},
		{"mixed units", "1d12h", 36 * time.Hour},
		{"fractional days", "0.5d", 12 * time.Hour},
		{"surrounding spaces", "  7d  ", 7 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePeriod(tt.period)
			if err != nil {
				t.Fatalf("ParsePeriod(%q) returned an unexpected error: %v", tt.period, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePeriod(%q) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestParsePeriodInvalid(t *testing.T) {
	invalid := []string{"xyz", "0", "-7d", "7days", "7 d", "d", "-1h", "0h", "7dd"}

	for _, period := range invalid {
		t.Run(period, func(t *testing.T) {
			if _, err := ParsePeriod(period); err == nil {
				t.Fatalf("ParsePeriod(%q) succeeded, want a configuration error", period)
			}
		})
	}
}
