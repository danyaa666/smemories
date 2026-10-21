package ulid

import (
	"regexp"
	"strings"
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

func TestValid(t *testing.T) {
	const ok = "01ARYZ6S41TSV4RRFFQ69G5FAV"
	if !Valid(ok) || !Valid(New(time.Now())) {
		t.Fatal("a canonical id was refused")
	}
	for name, s := range map[string]string{
		"empty":                         "",
		"lower case":                    strings.ToLower(ok),
		"mixed case":                    ok[:13] + strings.ToLower(ok[13:]),
		"25 characters":                 ok[:25],
		"27 characters":                 ok + "A",
		"I":                             "I" + ok[1:],
		"L":                             "L" + ok[1:],
		"O":                             "O" + ok[1:],
		"U":                             "U" + ok[1:],
		"space":                         " " + ok[1:],
		"trailing space":                ok[:25] + " ",
		"trailing newline":              ok[:25] + "\n",
		"nul":                           ok[:25] + "\x00",
		"accented look-alike (E-acute)": "\u00c9" + ok[2:], // 2 bytes: the total is 26 bytes, so only the alphabet check refuses it
		"fullwidth digit":               "\uff10" + ok[3:],
		"slash":                         ok[:25] + "/",
		"dot dot":                       "../" + ok[3:],
	} {
		if Valid(s) {
			t.Errorf("%s: %q accepted", name, s)
		}
	}
}
