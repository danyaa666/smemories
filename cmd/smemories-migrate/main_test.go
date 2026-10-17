package main

import "testing"

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		cmd  string
		ok   bool
	}{
		{[]string{"up"}, "up", true},
		{[]string{"status"}, "status", true},
		{[]string{"version"}, "version", true},
		{[]string{"down", "--allow-down"}, "down", true},
		{[]string{"down"}, "", false},
		{[]string{"up", "--allow-down"}, "", false},
		{[]string{"up", "x"}, "", false},
		{[]string{"sideways"}, "", false},
		{nil, "", false},
	} {
		cmd, _, ok := parseArgs(tc.args)
		if ok != tc.ok || (ok && cmd != tc.cmd) {
			t.Errorf("parseArgs(%v) = %q, %v; want %q, %v", tc.args, cmd, ok, tc.cmd, tc.ok)
		}
	}
}
