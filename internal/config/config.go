package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the patchwork.yaml configuration.
type Config struct {
	Instance    Instance    `yaml:"instance"`
	SMTP        SMTP        `yaml:"smtp"`
	Geographic  Geographic  `yaml:"geographic"`
	Modules     Modules     `yaml:"modules"`
	Branding    Branding    `yaml:"branding"`
	MultiQuilt  bool        `yaml:"multi_quilt"`
	Federation  Federation  `yaml:"federation"`
	Submissions Submissions `yaml:"submissions"`
	Server      Server      `yaml:"server"`
	Database    Database    `yaml:"database"`
	Session     Session     `yaml:"session"`
}

// Session bounds how long a signed-in session lives (docs/adr/017). Sessions
// stay deliberately long — volunteer organizers check in from a phone every
// few weeks, and on an SMTP-less instance re-authenticating can mean chasing
// down an invite link — so the safety comes from the idle timeout and from
// step-up auth on destructive actions, not from a short ceiling.
type Session struct {
	// MaxLifetime is the absolute ceiling. A session dies this long after it
	// was created no matter how active it is, so every session eventually
	// ends. Duration string; a bare "d" suffix means days ("30d", "720h").
	MaxLifetime string `yaml:"max_lifetime"`

	// IdleTimeout closes a session that has gone unused. A session dies at
	// whichever comes first, the absolute ceiling or last use plus this.
	IdleTimeout string `yaml:"idle_timeout"`
}

// Session lifetime defaults, used when patchwork.yaml says nothing. These
// reproduce the behaviour that was hardcoded before ADR 017: 30 days
// absolute. The 14-day idle timeout is a guess and should be revisited once
// a real instance has usage data.
const (
	DefaultSessionMaxLifetime = 30 * 24 * time.Hour
	DefaultSessionIdleTimeout = 14 * 24 * time.Hour
)

// Durations parses the configured strings, falling back to the defaults when
// a field is blank.
func (s Session) Durations() (maxLifetime, idleTimeout time.Duration, err error) {
	maxLifetime, err = parseDuration(s.MaxLifetime, DefaultSessionMaxLifetime)
	if err != nil {
		return 0, 0, fmt.Errorf("session.max_lifetime: %w", err)
	}
	idleTimeout, err = parseDuration(s.IdleTimeout, DefaultSessionIdleTimeout)
	if err != nil {
		return 0, 0, fmt.Errorf("session.idle_timeout: %w", err)
	}
	if maxLifetime <= 0 {
		return 0, 0, fmt.Errorf("session.max_lifetime must be positive")
	}
	if idleTimeout <= 0 {
		return 0, 0, fmt.Errorf("session.idle_timeout must be positive")
	}
	// An idle timeout longer than the ceiling can never fire, which is a
	// config that says one thing and does another. Say so rather than
	// silently ignoring it.
	if idleTimeout > maxLifetime {
		return 0, 0, fmt.Errorf("session.idle_timeout (%s) is longer than session.max_lifetime (%s), so it could never take effect", idleTimeout, maxLifetime)
	}
	return maxLifetime, idleTimeout, nil
}

// parseDuration accepts Go duration syntax plus a "d" (days) suffix, because
// session lifetimes are naturally written in days and "720h" is not a number
// anyone should have to work out.
func parseDuration(s string, fallback time.Duration) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(strings.TrimSpace(days), 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number of days", s)
		}
		return time.Duration(n * float64(24*time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration (try \"30d\", \"12h\", \"90m\")", s)
	}
	return d, nil
}

type Submissions struct {
	Enabled     bool `yaml:"enabled"`
	AutoApprove bool `yaml:"auto_approve"`
}

type Federation struct {
	Enabled bool `yaml:"enabled"`
}

type Instance struct {
	Name        string `yaml:"name"`
	Domain      string `yaml:"domain"`
	Description string `yaml:"description"`
}

type Branding struct {
	Color   string `yaml:"color"`
	LogoURL string `yaml:"logo_url"`
}

type SMTP struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
	From string `yaml:"from"`
}

// Configured returns true if SMTP has at minimum a host set.
func (s SMTP) Configured() bool {
	return s.Host != ""
}

type Geographic struct {
	Latitude  float64 `yaml:"latitude"`
	Longitude float64 `yaml:"longitude"`
	Radius    float64 `yaml:"radius"`
}

type Modules struct {
	Map        bool `yaml:"map"`
	Governance bool `yaml:"governance"`
	Ledger     bool `yaml:"ledger"`
}

type Server struct {
	Port string `yaml:"port"`

	// TrustedProxies lists CIDR blocks whose X-Forwarded-For headers are
	// honoured. Requests arriving from anywhere else have the header ignored
	// entirely and are attributed to their transport-level peer address.
	// Empty means the defaults in middleware.DefaultTrustedProxies (loopback
	// plus private ranges), which cover the bundled Docker Compose topology
	// where Caddy reaches the app over a private bridge network.
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type Database struct {
	Path string `yaml:"path"`
}

// Load reads and parses a patchwork.yaml file. It applies sensible defaults
// for optional fields.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file %q not found — copy patchwork.yaml.example to patchwork.yaml and edit it for your community", path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		Server:   Server{Port: "8080"},
		Database: Database{Path: "data/patchwork.db"},
		Modules: Modules{
			Map:        true,
			Governance: true,
			Ledger:     false,
		},
		Submissions: Submissions{
			Enabled:     true,
			AutoApprove: false,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Instance.Name == "" {
		return nil, fmt.Errorf("instance.name is required")
	}

	if cfg.Federation.Enabled && cfg.Instance.Domain == "" {
		return nil, fmt.Errorf("instance.domain is required when federation is enabled")
	}

	// Fail at startup rather than at the first login attempt.
	if _, _, err := cfg.Session.Durations(); err != nil {
		return nil, err
	}

	// Secrets can come from the environment so they don't have to live in the
	// YAML file (e.g. docker compose env_file).
	if pass := os.Getenv("PATCHWORK_SMTP_PASS"); pass != "" {
		cfg.SMTP.Pass = pass
	}

	// Port and database path can also come from the environment, so one
	// command can run an isolated instance without editing patchwork.yaml —
	// which is per-checkout and gitignored, so a test harness has no
	// business rewriting it. The e2e suite uses both to stand up a stack
	// that shares neither a port nor a database with a running dev server
	// (see web/e2e/ports.js).
	if port := os.Getenv("PATCHWORK_PORT"); port != "" {
		cfg.Server.Port = port
	}
	if dbPath := os.Getenv("PATCHWORK_DB_PATH"); dbPath != "" {
		cfg.Database.Path = dbPath
	}

	return cfg, nil
}

// exampleDomain is the placeholder shipped in patchwork.yaml.example.
const exampleDomain = "patchwork.example.com"

// Warnings returns human-readable notes about config values that look like
// they were never customized or will break things in production. They are
// logged at startup, not fatal.
func (c *Config) Warnings() []string {
	var w []string
	if c.Instance.Domain == exampleDomain {
		w = append(w, "instance.domain still has the example value — set it to your real domain")
	}
	if c.Federation.Enabled {
		d := c.Instance.Domain
		if d == exampleDomain || d == "localhost" || strings.HasPrefix(d, "localhost:") || strings.HasPrefix(d, "127.0.0.1") {
			w = append(w, fmt.Sprintf("federation is enabled but instance.domain is %q — remote instances cannot reach this address, and ActivityPub IDs minted with it are permanent", d))
		}
	}
	return w
}
