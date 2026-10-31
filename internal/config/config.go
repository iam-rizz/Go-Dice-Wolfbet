// Package config resolves CLI flags, environment variables and an
// optional TOML config file. Precedence: flags > env > TOML > defaults.
// Secrets (token, user_hash) must never be committed.
package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Defaults.
const (
	DefaultBaseURL  = "https://wolfbet.com"
	DefaultCurrency = "trx"
	DefaultMaxBets  = 0 // unlimited
)

// Config is the resolved run configuration.
type Config struct {
	BaseURL  string
	Token    string
	UserHash string
	Currency string
	Script   string // path to the .lua script
	DryRun   bool
	Seed     int64 // dry-run RNG seed (0 = fixed default)

	StopOnWin  bool
	StopOnLoss bool
	MaxBets    int

	ConfigFile string
}

// FromEnv are the supported environment variables.
const (
	EnvToken    = "WDBGO_TOKEN"
	EnvUserHash = "WDBGO_USER_HASH"
)

// Parse builds the config from args (normally os.Args[1:]). Returns the
// config and the parsed flag set (errors already consumed) — usage
// errors are reported as error values.
func Parse(args []string) (*Config, error) {
	cfg := &Config{}

	fs := flag.NewFlagSet("wdbgo", flag.ContinueOnError)
	fs.StringVar(&cfg.ConfigFile, "config", "", "TOML config file (token, user_hash, currency)")
	fs.StringVar(&cfg.Token, "token", "", "wolfbet bearer token (env "+EnvToken+")")
	fs.StringVar(&cfg.UserHash, "user-hash", "", "wolfbet x-hash-api value (env "+EnvUserHash+")")
	fs.StringVar(&cfg.Currency, "currency", DefaultCurrency, "coin to bet (e.g. trx)")
	fs.StringVar(&cfg.Script, "script", "", "path to the .lua user script")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "paper trading: simulate rolls, no real bets")
	fs.Int64Var(&cfg.Seed, "seed", 42, "dry-run RNG seed")
	fs.BoolVar(&cfg.StopOnWin, "stop-on-win", false, "stop after the next win")
	fs.BoolVar(&cfg.StopOnLoss, "stop-on-loss", false, "stop after the next loss")
	fs.IntVar(&cfg.MaxBets, "max-bets", DefaultMaxBets, "stop after N bets (0 = unlimited)")
	fs.StringVar(&cfg.BaseURL, "base-url", DefaultBaseURL, "wolfbet base URL")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// track explicitly-set flags so lower-precedence layers never
	// override them
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// TOML file first (lowest precedence)
	if cfg.ConfigFile != "" {
		if err := applyToml(cfg, cfg.ConfigFile, set); err != nil {
			return nil, err
		}
	}

	// env overrides TOML, but never an explicit flag
	if v := os.Getenv(EnvToken); v != "" && !set["token"] {
		cfg.Token = v
	}
	if v := os.Getenv(EnvUserHash); v != "" && !set["user-hash"] {
		cfg.UserHash = v
	}

	return cfg, cfg.Validate()
}

// Validate enforces the hard requirements.
func (c *Config) Validate() error {
	if c.Script == "" {
		return fmt.Errorf("--script is required (path to a .lua file)")
	}
	if _, err := os.Stat(c.Script); err != nil {
		return fmt.Errorf("script file: %w", err)
	}
	if !c.DryRun {
		if c.Token == "" {
			return fmt.Errorf("token is required (or use --dry-run): --token, %s or TOML config", EnvToken)
		}
		if c.UserHash == "" {
			return fmt.Errorf("user-hash is required (or use --dry-run): --user-hash, %s or TOML config", EnvUserHash)
		}
	}
	if c.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if c.MaxBets < 0 {
		return fmt.Errorf("--max-bets must be >= 0")
	}
	return nil
}

// applyToml reads a flat `key = "value"` TOML subset (comments with #,
// quoted strings, ints, bools). Enough for token/user_hash/currency —
// no tables, no arrays. Explicitly-set flags are never overridden.
func applyToml(cfg *Config, path string, set map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return fmt.Errorf("config file %s: invalid line %q", path, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if i := strings.Index(val, " #"); i >= 0 && !strings.HasPrefix(val, "\"") {
			val = strings.TrimSpace(val[:i])
		}
		val = strings.Trim(val, "\"'")

		switch key {
		case "token":
			if !set["token"] {
				cfg.Token = val
			}
		case "user_hash":
			if !set["user-hash"] {
				cfg.UserHash = val
			}
		case "currency":
			if !set["currency"] {
				cfg.Currency = val
			}
		case "base_url":
			if !set["base-url"] {
				cfg.BaseURL = val
			}
		case "max_bets":
			if !set["max-bets"] {
				if n, err := strconv.Atoi(val); err == nil {
					cfg.MaxBets = n
				}
			}
		default:
			// unknown keys are ignored (forward compatible)
		}
	}
	return scanner.Err()
}
