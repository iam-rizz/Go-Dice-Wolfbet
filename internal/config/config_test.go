package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeScript(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "script.lua")
	if err := os.WriteFile(p, []byte("function dobet() end"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFlagsHappyPath(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	cfg, err := Parse([]string{
		"--script", script,
		"--token", "tok",
		"--user-hash", "hash",
		"--currency", "LTC",
		"--max-bets", "20",
		"--stop-on-win",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "tok" || cfg.UserHash != "hash" {
		t.Errorf("token/user_hash = %q/%q", cfg.Token, cfg.UserHash)
	}
	if cfg.Currency != "LTC" {
		t.Errorf("currency = %q", cfg.Currency)
	}
	if cfg.MaxBets != 20 || !cfg.StopOnWin || cfg.StopOnLoss {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("base url = %q", cfg.BaseURL)
	}
}

func TestScriptRequired(t *testing.T) {
	if _, err := Parse([]string{"--token", "t", "--user-hash", "h"}); err == nil {
		t.Fatal("missing --script must fail")
	}
}

func TestScriptMustExist(t *testing.T) {
	if _, err := Parse([]string{"--script", "/nonexistent/x.lua", "--token", "t", "--user-hash", "h"}); err == nil {
		t.Fatal("nonexistent script must fail")
	}
}

func TestLiveNeedsTokenAndHash(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	// no token → error
	if _, err := Parse([]string{"--script", script}); err == nil {
		t.Fatal("live mode without token must fail")
	}
	// token but no hash → error
	if _, err := Parse([]string{"--script", script, "--token", "t"}); err == nil {
		t.Fatal("live mode without user-hash must fail")
	}
	// dry-run skips both
	cfg, err := Parse([]string{"--script", script, "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DryRun {
		t.Error("dry-run flag lost")
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	t.Setenv(EnvToken, "env-tok")
	t.Setenv(EnvUserHash, "env-hash")
	cfg, err := Parse([]string{"--script", script})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-tok" || cfg.UserHash != "env-hash" {
		t.Errorf("env not applied: %q/%q", cfg.Token, cfg.UserHash)
	}
}

func TestTomlConfig(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	tomlPath := filepath.Join(dir, "wdbgo.toml")
	toml := `
# wdbgo config
token = "toml-tok"
user_hash = "toml-hash"
currency = "doge"
`
	if err := os.WriteFile(tomlPath, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse([]string{"--script", script, "--config", tomlPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "toml-tok" || cfg.UserHash != "toml-hash" {
		t.Errorf("toml not applied: %q/%q", cfg.Token, cfg.UserHash)
	}
	if cfg.Currency != "doge" {
		t.Errorf("currency = %q", cfg.Currency)
	}
}

func TestEnvBeatsToml(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	tomlPath := filepath.Join(dir, "wdbgo.toml")
	if err := os.WriteFile(tomlPath, []byte("token = \"toml-tok\"\nuser_hash = \"h\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvToken, "env-tok")
	cfg, err := Parse([]string{"--script", script, "--config", tomlPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-tok" {
		t.Errorf("token = %q, env must win over toml", cfg.Token)
	}
}

func TestFlagBeatsEnv(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	t.Setenv(EnvToken, "env-tok")
	cfg, err := Parse([]string{"--script", script, "--token", "flag-tok", "--user-hash", "h"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "flag-tok" {
		t.Errorf("token = %q, flag must win over env", cfg.Token)
	}
}

func TestMaxBetsNegative(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	if _, err := Parse([]string{"--script", script, "--token", "t", "--user-hash", "h", "--max-bets", "-1"}); err == nil {
		t.Fatal("negative max-bets must fail")
	}
}

func TestCasinoDefaultsToWolfbet(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	cfg, err := Parse([]string{"--script", script, "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Casino != CasinoWolfbet {
		t.Errorf("casino = %q, want %q", cfg.Casino, CasinoWolfbet)
	}
}

func TestStakeLiveNeedsSession(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	// stake live without session → error
	if _, err := Parse([]string{"--script", script, "--casino", "stake"}); err == nil {
		t.Fatal("stake live mode without session must fail")
	}
	// with session → ok, and wolfbet creds are not required
	cfg, err := Parse([]string{"--script", script, "--casino", "stake", "--session", "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Session != "sess-1" {
		t.Errorf("session = %q", cfg.Session)
	}
	// dry-run skips the session requirement
	if _, err := Parse([]string{"--script", script, "--casino", "stake", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownCasinoRejected(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	if _, err := Parse([]string{"--script", script, "--casino", "roobet"}); err == nil {
		t.Fatal("unknown casino must fail")
	}
}

func TestSessionFromEnvAndToml(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir)
	t.Setenv(EnvSession, "env-sess")
	cfg, err := Parse([]string{"--script", script, "--casino", "stake"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Session != "env-sess" {
		t.Errorf("session = %q, want env-sess", cfg.Session)
	}

	tomlPath := filepath.Join(dir, "wdbgo.toml")
	if err := os.WriteFile(tomlPath, []byte("session = \"toml-sess\"\ncasino = \"stake\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv(EnvSession)
	cfg2, err := Parse([]string{"--script", script, "--config", tomlPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Session != "toml-sess" {
		t.Errorf("session = %q, want toml-sess", cfg2.Session)
	}
	if cfg2.Casino != CasinoStake {
		t.Errorf("casino = %q, want stake", cfg2.Casino)
	}
}
