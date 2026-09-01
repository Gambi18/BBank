// Package platform holds process-level concerns: configuration, logging setup,
// auth keys, and other infrastructure that is not domain logic.
package platform

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds every value the process reads from the environment.
type Config struct {
	DatabaseURL    string
	Port           string
	AllowedOrigins []string
	LogLevel       slog.Level
	ShutdownGrace  time.Duration
}

// Load reads and validates configuration. Missing required values are a startup
// failure, never a silent guess (WI-07).
func Load() (Config, error) {
	c := Config{Port: envOr("PORT", "8000"), ShutdownGrace: 20 * time.Second}

	// WI-07: no fallback DSN. The old hardcoded localhost default meant a
	// misconfigured deployment silently pointed at the wrong database.
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required (see .env.example); refusing to guess a connection string")
	}

	// WI-03: CORS is an explicit allowlist. Empty means no cross-origin browser
	// access, which is correct for the server-to-server topology we actually use.
	if raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}

	switch strings.ToLower(envOr("LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		c.LogLevel = slog.LevelInfo
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// safeDSN renders a connection string for logs with the password removed.
// WI-01: the previous code printed the raw DSN, password included, on every boot.
// SafeDSN renders a connection string for logs with the password removed (WI-01).
func SafeDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "(unparseable DSN)"
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
	}
	return u.Redacted()
}
