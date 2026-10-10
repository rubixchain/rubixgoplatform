package logrotate

import "time"

// NextBoundary returns the first rotation instant strictly after now.
//
// Every period is anchored on the local midnight of the current day and
// stepped by the period, e.g. 30m rotates at :00 and :30 of every hour, 12h
// rotates at 00:00 and 12:00, 24h rotates at midnight and 7d rotates at the
// midnight which follows a whole week.
func NextBoundary(now time.Time, period time.Duration) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	elapsed := now.Sub(midnight)
	next := midnight.Add((elapsed/period + 1) * period)

	for !next.After(now) {
		next = next.Add(period)
	}

	return next
}

// PreviousBoundary returns the rotation instant which precedes now. It is used
// to infer the start of the period covered by an already existing log file.
func PreviousBoundary(now time.Time, period time.Duration) time.Time {
	return NextBoundary(now, period).Add(-period)
}
