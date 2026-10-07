package ulid

import (
	"regexp"
	"testing"
	"time"
)

var shape = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func TestShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := New(time.Now())
		if !shape.MatchString(id) || seen[id] {
			t.Fatalf("bad or repeated id %q", id)
		}
		seen[id] = true
	}
}

func TestTimestampPrefixKnownValue(t *testing.T) {
	// 2016-07-30T23:54:10.259Z is the timestamp in the ULID spec's example (01ARYZ6S41...).
	got := New(time.UnixMilli(1469918176385))[:10]
	if got != "01ARYZ6S41" {
		t.Fatalf("timestamp part = %q, want 01ARYZ6S41", got)
	}
	if a, b := New(time.UnixMilli(1000))[:10], New(time.UnixMilli(2000))[:10]; a >= b {
		t.Fatalf("not sortable: %s %s", a, b)
	}
}

func TestPreEpochTimeClampsToZero(t *testing.T) {
	if got := New(time.UnixMilli(-1))[:10]; got != "0000000000" {
		t.Fatalf("pre-1970 time should clamp to 0, got prefix %q", got)
	}
}
