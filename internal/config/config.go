// Package config reads the process configuration from SMEM_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
)

// Config is the validated process configuration.
type Config struct {
	HTTPAddr string     // SMEM_HTTP_ADDR, host:port to listen on
	Env      string     // SMEM_ENV: dev, test or prod
	LogLevel slog.Level // SMEM_LOG_LEVEL: debug, info, warn or error
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Load builds a Config from getenv (os.Getenv in production). An unset or empty
// variable takes its default; an invalid value returns an error naming the variable.
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	cfg := Config{HTTPAddr: get("SMEM_HTTP_ADDR", ":8080"), Env: get("SMEM_ENV", "dev")}

	if _, port, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return Config{}, fmt.Errorf("SMEM_HTTP_ADDR=%q: want host:port, e.g. :8080", cfg.HTTPAddr)
	} else if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return Config{}, fmt.Errorf("SMEM_HTTP_ADDR=%q: port must be a number 0-65535", cfg.HTTPAddr)
	}

	switch cfg.Env {
	case "dev", "test", "prod":
	default:
		return Config{}, fmt.Errorf("SMEM_ENV=%q: must be one of dev, test, prod", cfg.Env)
	}

	raw := get("SMEM_LOG_LEVEL", "info")
	lvl, ok := logLevels[raw]
	if !ok {
		return Config{}, fmt.Errorf("SMEM_LOG_LEVEL=%q: must be one of debug, info, warn, error", raw)
	}
	cfg.LogLevel = lvl
	return cfg, nil
}
