package logrotate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPeriod is used when `log_rotation_period` is absent from config.toml.
	DefaultPeriod = "7d"

	// WeeklyPeriod is the duration which DefaultPeriod resolves to.
	WeeklyPeriod = 7 * 24 * time.Hour
)

// dayExpr matches a `<number>d` term of a duration string. Go's duration units
// (ns, us, µs, ms, s, m, h) never contain a `d`, so the match is unambiguous.
var dayExpr = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)d`)

// ParsePeriod parses a rotation period from config.toml. It accepts every unit
// supported by time.ParseDuration and additionally the `d` (day) suffix, so
// "7d", "24h", "12h", "30m" and mixed forms such as "1d12h" are all valid.
//
// An empty value falls back to DefaultPeriod. Values which are not a valid
// duration, or which are zero or negative, are reported as an error so that a
// misconfigured node never silently rotates on an unexpected interval.
func ParsePeriod(period string) (time.Duration, error) {
	p := strings.TrimSpace(period)
	if p == "" {
		p = DefaultPeriod
	}

	expanded, err := expandDays(p)
	if err != nil {
		return 0, err
	}

	d, err := time.ParseDuration(expanded)
	if err != nil {
		return 0, fmt.Errorf("invalid log_rotation_period %q: %v", period, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid log_rotation_period %q: must be greater than zero", period)
	}

	return d, nil
}

// expandDays rewrites every `<number>d` term into its equivalent in hours so
// that the result can be handed over to time.ParseDuration.
func expandDays(period string) (string, error) {
	var convErr error

	expanded := dayExpr.ReplaceAllStringFunc(period, func(match string) string {
		days, err := strconv.ParseFloat(strings.TrimSuffix(match, "d"), 64)
		if err != nil {
			convErr = fmt.Errorf("invalid log_rotation_period %q: %v", period, err)
			return match
		}
		return strconv.FormatFloat(days*24, 'f', -1, 64) + "h"
	})
	if convErr != nil {
		return "", convErr
	}

	return expanded, nil
}
