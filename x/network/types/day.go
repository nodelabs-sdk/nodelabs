package types

import "time"

// secondsPerDay is the length of a UTC day bucket.
const secondsPerDay = 24 * 60 * 60

// DayEpoch returns the number of whole UTC days since the Unix epoch for t.
// It is the day-bucket key used by the recent-activity index and the daily
// counters.
func DayEpoch(t time.Time) uint64 {
	return uint64(t.Unix() / secondsPerDay)
}

// SameDay reports whether a and b fall in the same UTC day bucket.
func SameDay(a, b time.Time) bool {
	return DayEpoch(a) == DayEpoch(b)
}

// LookbackStart returns midnight UTC of the day before blockTime: the start
// of the two-day window (yesterday + today) that recent-activity checks
// cover.
func LookbackStart(blockTime time.Time) time.Time {
	return time.Unix(int64(DayEpoch(blockTime)-1)*secondsPerDay, 0).UTC()
}
