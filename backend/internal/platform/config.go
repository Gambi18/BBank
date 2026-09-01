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

	// Auth (WI-17). The private key signs; the frontend only ever gets the public
	// half via GET /api/v1/auth/public-key.
	JWTPrivateKey     string
	JWTIssuer         string
	JWTAudience       string
	AllowEphemeralKey bool // dev only: generate a key if none is supplied
	CookieSecure      bool
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

	c.JWTPrivateKey = os.Getenv("JWT_PRIVATE_KEY")
	c.JWTIssuer = envOr("JWT_ISSUER", "https://api.bbank.local")
	c.JWTAudience = envOr("JWT_AUDIENCE", "bbank-web")
	c.AllowEphemeralKey = envOr("ALLOW_EPHEMERAL_JWT_KEY", "false") == "true"
	c.CookieSecure = envOr("COOKIE_SECURE", "true") == "true"

	// Refusing to start beats silently signing with a key that dies on restart.
	if c.JWTPrivateKey == "" && !c.AllowEphemeralKey {
		return c, errors.New("JWT_PRIVATE_KEY is required (or set ALLOW_EPHEMERAL_JWT_KEY=true for local development only)")
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
