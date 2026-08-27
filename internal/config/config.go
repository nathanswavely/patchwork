package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	// The distroless image ships no tzdata, and a configured zone name
	// has to resolve wherever the binary runs.
	_ "time/tzdata"
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

	// Timezone is the deprecated home of the community's zone. It moved
	// to geographic.timezone, beside the coordinates that already declare
	// where this quilt is (docs/adr/045, docs/adr/067). Still read, so an
	// instance that set it during the ADR 065 window keeps working; the
	// geographic key wins where both are present, and Warnings says so.
	Timezone string `yaml:"timezone"`

	// BootstrapToken gates the very first account (docs/adr/070). The first
	// account becomes instance admin, and magic-link signup is open
	// registration, so without this an instance reachable before its
	// operator signs up is claimable by whoever finds it first. Leave it
	// unset and the server generates one at startup and prints it in the
	// first-run notice; a provisioning layer sets it and hands it to the
	// operator. Dead the moment an account exists.
	BootstrapToken string `yaml:"bootstrap_token"`
}

// Timezone is the quilt's configured zone name, the bootstrap default the
// instance_settings override is layered over. geographic.timezone is the
// home; instance.timezone is the deprecated spelling and loses to it.
func (c *Config) Timezone() string {
	if tz := strings.TrimSpace(c.Geographic.Timezone); tz != "" {
		return tz
	}
	return strings.TrimSpace(c.Instance.Timezone)
}

// Location resolves the configured zone. Load has already validated it,
// so a caller can take the location and not the error; an unset or
// unparseable zone is UTC.
func (c *Config) Location() *time.Location {
	loc, err := parseTimezone(c.Timezone())
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseTimezone resolves an IANA timezone name. Empty is UTC.
func parseTimezone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%q is not an IANA timezone name (try \"America/New_York\", \"Europe/Berlin\", \"UTC\")", name)
	}
	return loc, nil
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

	// Timezone is the IANA name of the zone this community keeps time in
	// ("America/New_York"). It lives here because it is the same claim
	// the coordinates above make — where this quilt is — and it is the
	// bottom of the chain an event's zone resolves through: event →
	// patch → instance → UTC (docs/adr/045).
	//
	// It is the bootstrap default only. An admin can override it from the
	// admin panel without a redeploy, the way instance.name already
	// works, and the override wins.
	//
	// Empty means UTC, which is right only for a community that keeps
	// UTC. Anywhere else it renders every event hours off, and reads
	// zoneless calendar feeds hours off on the way in.
	Timezone string `yaml:"timezone"`
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

	// A typo here is invisible until an imported calendar is hours off,
	// so it fails at startup like the session durations below.
	if _, err := parseTimezone(cfg.Geographic.Timezone); err != nil {
		return nil, fmt.Errorf("geographic.timezone: %w", err)
	}
	if _, err := parseTimezone(cfg.Instance.Timezone); err != nil {
		return nil, fmt.Errorf("instance.timezone: %w", err)
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
	if tok := os.Getenv("PATCHWORK_BOOTSTRAP_TOKEN"); tok != "" {
		cfg.Instance.BootstrapToken = tok
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
	if c.Timezone() == "" {
		w = append(w, "geographic.timezone is unset, so every event renders in each reader's own browser zone and zoneless calendar feeds are read as UTC — set it to your community's IANA zone (e.g. America/New_York)")
	}
	if strings.TrimSpace(c.Instance.Timezone) != "" {
		if strings.TrimSpace(c.Geographic.Timezone) != "" {
			w = append(w, "instance.timezone and geographic.timezone are both set — geographic.timezone wins; delete instance.timezone")
		} else {
			w = append(w, "instance.timezone is deprecated — move it to geographic.timezone, beside latitude and longitude")
		}
	}
	if c.Federation.Enabled {
		d := c.Instance.Domain
		if d == exampleDomain || d == "localhost" || strings.HasPrefix(d, "localhost:") || strings.HasPrefix(d, "127.0.0.1") {
			w = append(w, fmt.Sprintf("federation is enabled but instance.domain is %q — remote instances cannot reach this address, and ActivityPub IDs minted with it are permanent", d))
		}
	}
	return w
}
