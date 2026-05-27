// Package config parses environment variables into a typed Config struct.
// Called once from main; the result is passed explicitly — no globals.
package config

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadDotEnv looks for a `.env` file in the current working directory and
// imports KEY=VALUE lines into the process environment. Existing env vars are
// NOT overwritten — the real environment always wins. Comments (#) and blank
// lines are skipped. Surrounding ASCII quotes are stripped from the value.
//
// Returns the path that was loaded (empty if no .env was found) and any
// parse error.
func LoadDotEnv() (string, error) {
	const path = ".env"
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	lineno := 0
	for s.Scan() {
		lineno++
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return path, fmt.Errorf(".env:%d: not a KEY=VALUE line", lineno)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// strip inline trailing comment if value isn't quoted
		if !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `'`) {
			if hash := strings.Index(val, " #"); hash >= 0 {
				val = strings.TrimSpace(val[:hash])
			}
		}
		// strip matched surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, set := os.LookupEnv(key); set {
			continue // never overwrite real env
		}
		_ = os.Setenv(key, val)
	}
	if err := s.Err(); err != nil {
		return path, err
	}
	return path, nil
}

type Config struct {
	ListenAddr           string
	PublicBaseURL        string
	LogLevel             string
	TrustedProxy         bool
	TrustedProxyCIDRs    []netip.Prefix // if non-empty, X-Authentik-* only honored when RemoteAddr matches
	DatabaseURL          string
	DefaultTTL           time.Duration
	MaxCiphertextBytes   int
	RatePerMin           int
	RateBurst            int
	NotifyWebhookURL     string
	NotifyWebhookTimeout time.Duration
	DevUser              string // empty unless GOGRAB_DEV_USER is set
	MigrateOnBoot        bool   // GOGRAB_MIGRATE_ON_BOOT=1 → goose up before listening

	// WebAuthn / session unlock
	RPDisplayName string   // "GoGrab"
	RPID          string   // e.g. "gograb.example.com" or "localhost" for dev
	RPOrigins     []string // e.g. ["https://gograb.example.com"] or dev origins
	SessionSecret []byte   // 32 random bytes for WebAuthn session-token AES key
}

func Load() (Config, error) {
	c := Config{
		ListenAddr:           env("GOGRAB_LISTEN_ADDR", ":8080"),
		PublicBaseURL:        env("GOGRAB_PUBLIC_BASE_URL", "http://localhost:8080"),
		LogLevel:             env("GOGRAB_LOG_LEVEL", "info"),
		DatabaseURL:          env("GOGRAB_DATABASE_URL", ""),
		NotifyWebhookURL:     env("GOGRAB_NOTIFY_WEBHOOK_URL", ""),
		DevUser:              env("GOGRAB_DEV_USER", ""),
	}
	var err error
	if c.TrustedProxy, err = envBool("GOGRAB_TRUSTED_PROXY", true); err != nil {
		return c, err
	}
	if c.MigrateOnBoot, err = envBool("GOGRAB_MIGRATE_ON_BOOT", false); err != nil {
		return c, err
	}
	if raw := os.Getenv("GOGRAB_TRUSTED_PROXY_CIDRS"); raw != "" {
		for _, cidr := range strings.Split(raw, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			p, err := netip.ParsePrefix(cidr)
			if err != nil {
				return c, fmt.Errorf("GOGRAB_TRUSTED_PROXY_CIDRS: %q is not a valid CIDR: %w", cidr, err)
			}
			c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, p)
		}
	}
	hours, err := envInt("GOGRAB_DEFAULT_TTL_HOURS", 72)
	if err != nil {
		return c, err
	}
	c.DefaultTTL = time.Duration(hours) * time.Hour
	if c.MaxCiphertextBytes, err = envInt("GOGRAB_MAX_CIPHERTEXT_BYTES", 65536); err != nil {
		return c, err
	}
	if c.RatePerMin, err = envInt("GOGRAB_RATE_PER_MIN", 20); err != nil {
		return c, err
	}
	if c.RateBurst, err = envInt("GOGRAB_RATE_BURST", 5); err != nil {
		return c, err
	}
	secs, err := envInt("GOGRAB_NOTIFY_WEBHOOK_TIMEOUT_SECONDS", 5)
	if err != nil {
		return c, err
	}
	c.NotifyWebhookTimeout = time.Duration(secs) * time.Second

	// WebAuthn relying party. RPID must match the registrable domain. For dev
	// (vite at localhost:5173 proxying to :8080) RPID="localhost" works.
	c.RPDisplayName = env("GOGRAB_RP_DISPLAY_NAME", "GoGrab")
	c.RPID = env("GOGRAB_RP_ID", "localhost")
	if origins := env("GOGRAB_RP_ORIGINS", "http://localhost:5173,http://localhost:8080"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				c.RPOrigins = append(c.RPOrigins, o)
			}
		}
	}

	// 32-byte session secret. If unset, a random one is generated per process —
	// in-flight WebAuthn ceremonies don't survive a restart, which is fine
	// because the begin/finish window is seconds long.
	if v := os.Getenv("GOGRAB_SESSION_SECRET"); v != "" {
		raw, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil {
			raw, err = base64.StdEncoding.DecodeString(v)
		}
		if err != nil {
			return c, fmt.Errorf("GOGRAB_SESSION_SECRET: invalid base64: %w", err)
		}
		if len(raw) != 32 {
			return c, fmt.Errorf("GOGRAB_SESSION_SECRET: need 32 bytes, got %d", len(raw))
		}
		c.SessionSecret = raw
	} else {
		c.SessionSecret = make([]byte, 32)
		if _, err := rand.Read(c.SessionSecret); err != nil {
			return c, fmt.Errorf("gen session secret: %w", err)
		}
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("GOGRAB_DATABASE_URL is required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("env %s: %w", k, err)
	}
	return n, nil
}

func envBool(k string, def bool) (bool, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("env %s: %w", k, err)
	}
	return b, nil
}
