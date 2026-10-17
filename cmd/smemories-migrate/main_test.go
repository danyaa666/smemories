package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

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

// runMigrateEnv makes the test binary run main() with the arguments in it, so the exit codes of docs/deploy.md are checked on the real
// command (os.Exit and all) without building the binary.
const runMigrateEnv = "SMEM_TEST_RUN_MIGRATE_ARGS"

func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv(runMigrateEnv); ok {
		os.Args = append([]string{"smemories-migrate"}, strings.Fields(args)...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// AC4 (T-090): the exit codes in the docs. In prod a bare `down` is a usage error (2); only `down --allow-down` reaches the prod refusal (4);
// `up --allow-down` is a usage error. None of them connects to anything.
func TestExitCodesInProd(t *testing.T) {
	for _, tc := range []struct {
		args string
		code int
	}{
		{"down", exitUsage},
		{"down --allow-down", exitRefused},
		{"up --allow-down", exitUsage},
		{"sideways", exitUsage},
	} {
		cmd := exec.Command(os.Args[0])
		cmd.Env = []string{
			runMigrateEnv + "=" + tc.args,
			"SMEM_ENV=prod", "SMEM_DB_DSN=u:p@tcp(127.0.0.1:1)/x", "SMEM_DB_ALLOW_INSECURE=true", "SMEM_S3_ALLOW_INSECURE=true",
			"SMEM_S3_BUCKET=b", "SMEM_S3_ENDPOINT=http://x:9000", "SMEM_S3_ACCESS_KEY=a", "SMEM_S3_SECRET_KEY=b",
		}
		err := cmd.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != tc.code {
			t.Errorf("smemories-migrate %s in prod: %v, want exit code %d", tc.args, err, tc.code)
		}
	}
}
