// Package config reads the process configuration from SMEM_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// Config is the validated process configuration.
type Config struct {
	HTTPAddr string     // SMEM_HTTP_ADDR, host:port to listen on
	Env      string     // SMEM_ENV: dev, test or prod
	LogLevel slog.Level // SMEM_LOG_LEVEL: debug, info, warn or error

	DBDSN             string        // SMEM_DB_DSN: user:pass@tcp(host:port)/dbname; required unless Env is "test" (db.Open still rejects an empty one). Never log it.
	DBMaxOpen         int           // SMEM_DB_MAX_OPEN, default 20
	DBMaxIdle         int           // SMEM_DB_MAX_IDLE, default 5
	DBConnMaxLifetime time.Duration // SMEM_DB_CONN_MAX_LIFETIME, default 5m
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

	var err error
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

	cfg.DBDSN = getenv("SMEM_DB_DSN")
	if cfg.DBDSN == "" && cfg.Env != "test" {
		return Config{}, fmt.Errorf("SMEM_DB_DSN is required (e.g. user:pass@tcp(127.0.0.1:3306)/smemories)")
	}
	if cfg.DBMaxOpen, err = getInt(get, "SMEM_DB_MAX_OPEN", "20", 1); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxIdle, err = getInt(get, "SMEM_DB_MAX_IDLE", "5", 0); err != nil {
		return Config{}, err
	}
	raw = get("SMEM_DB_CONN_MAX_LIFETIME", "5m")
	if cfg.DBConnMaxLifetime, err = time.ParseDuration(raw); err != nil || cfg.DBConnMaxLifetime < 0 {
		return Config{}, fmt.Errorf("SMEM_DB_CONN_MAX_LIFETIME=%q: want a non-negative duration such as 5m", raw)
	}
	return cfg, nil
}

func getInt(get func(key, def string) string, key, def string, min int) (int, error) {
	raw := get(key, def)
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return 0, fmt.Errorf("%s=%q: want an integer >= %d", key, raw, min)
	}
	return n, nil
}
