package logrotate

import (
	"testing"
	"time"
)

func at(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		t.Fatalf("failed to parse the test timestamp %q: %v", value, err)
	}
	return ts
}

func TestNextBoundaryDefaultPeriod(t *testing.T) {
	tests := []struct {
		name string
		now  string
		want string
	}{
		{"on a midnight boundary", "2026-08-24 00:00:00", "2026-08-31 00:00:00"},
		{"early monday", "2026-08-24 01:15:00", "2026-08-31 00:00:00"},
		{"wednesday", "2026-08-26 17:42:13", "2026-09-02 00:00:00"},
		{"friday", "2026-08-28 09:00:00", "2026-09-04 00:00:00"},
		{"sunday night", "2026-08-30 23:59:59", "2026-09-06 00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextBoundary(at(t, tt.now), WeeklyPeriod)
			if want := at(t, tt.want); !got.Equal(want) {
				t.Fatalf("NextBoundary(%v) = %v, want %v", tt.now, got, want)
			}
		})
	}
}

func TestNextBoundaryInterval(t *testing.T) {
	tests := []struct {
		name   string
		now    string
		period time.Duration
		want   string
	}{
		{"daily", "2026-08-26 17:42:13", 24 * time.Hour, "2026-08-27 00:00:00"},
		{"twelve hourly, morning", "2026-08-26 09:10:00", 12 * time.Hour, "2026-08-26 12:00:00"},
		{"twelve hourly, evening", "2026-08-26 13:10:00", 12 * time.Hour, "2026-08-27 00:00:00"},
		{"half hourly", "2026-08-26 09:10:00", 30 * time.Minute, "2026-08-26 09:30:00"},
		{"half hourly on the boundary", "2026-08-26 09:30:00", 30 * time.Minute, "2026-08-26 10:00:00"},
		{"hourly at midnight", "2026-08-26 00:00:00", time.Hour, "2026-08-26 01:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextBoundary(at(t, tt.now), tt.period)
			if want := at(t, tt.want); !got.Equal(want) {
				t.Fatalf("NextBoundary(%v, %v) = %v, want %v", tt.now, tt.period, got, want)
			}
		})
	}
}

func TestPreviousBoundary(t *testing.T) {
	tests := []struct {
		name   string
		now    string
		period time.Duration
		want   string
	}{
		{"weekly from a friday", "2026-08-28 09:00:00", WeeklyPeriod, "2026-08-28 00:00:00"},
		{"weekly just after midnight", "2026-08-24 00:00:01", WeeklyPeriod, "2026-08-24 00:00:00"},
		{"twelve hourly", "2026-08-26 13:10:00", 12 * time.Hour, "2026-08-26 12:00:00"},
		{"half hourly", "2026-08-26 09:10:00", 30 * time.Minute, "2026-08-26 09:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PreviousBoundary(at(t, tt.now), tt.period)
			if want := at(t, tt.want); !got.Equal(want) {
				t.Fatalf("PreviousBoundary(%v, %v) = %v, want %v", tt.now, tt.period, got, want)
			}
		})
	}
}

// A boundary is never in the past and never more than one period ahead.
func TestNextBoundaryIsWithinOnePeriod(t *testing.T) {
	periods := []time.Duration{WeeklyPeriod, 24 * time.Hour, 12 * time.Hour, 30 * time.Minute, 90 * time.Minute}
	now := at(t, "2026-08-26 17:42:13")

	for _, period := range periods {
		next := NextBoundary(now, period)
		if !next.After(now) {
			t.Fatalf("NextBoundary(%v) = %v is not after now", period, next)
		}
		if next.Sub(now) > period {
			t.Fatalf("NextBoundary(%v) = %v is more than one period ahead of now", period, next)
		}
	}
}
