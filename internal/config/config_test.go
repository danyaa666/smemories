package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.Env != "dev" || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_HTTP_ADDR": "127.0.0.1:0", "SMEM_ENV": "prod", "SMEM_LOG_LEVEL": "debug"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:0" || cfg.Env != "prod" || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadInvalidNamesVariable(t *testing.T) {
	cases := []struct{ key, val string }{
		{"SMEM_HTTP_ADDR", "nonsense"},
		{"SMEM_HTTP_ADDR", ":99999"},
		{"SMEM_HTTP_ADDR", ":http"},
		{"SMEM_ENV", "staging"},
		{"SMEM_ENV", "PROD"},
		{"SMEM_LOG_LEVEL", "verbose"},
		{"SMEM_LOG_LEVEL", "INFO"},
	}
	for _, c := range cases {
		_, err := Load(env(map[string]string{c.key: c.val}))
		if err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%q: want error naming the variable, got %v", c.key, c.val, err)
		}
	}
}
