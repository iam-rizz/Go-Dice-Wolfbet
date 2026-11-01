package login

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialsRoundTrip(t *testing.T) {
	// point the store at a temp dir
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	c := Credentials{Email: "me@example.com", Password: "s3cret"}
	if err := SaveCredentials(c); err != nil {
		t.Fatal(err)
	}

	// file must be owner-only
	info, err := os.Stat(filepath.Join(tmp, ".wdbgo", "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}

	got, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Errorf("loaded = %+v, want %+v", got, c)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if got := LoadSession(); got != "" {
		t.Errorf("LoadSession on empty = %q, want empty", got)
	}
	if err := SaveSession("abc123"); err != nil {
		t.Fatal(err)
	}
	if got := LoadSession(); got != "abc123" {
		t.Errorf("LoadSession = %q, want abc123", got)
	}
}

func TestLoadCredentialsMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if _, err := LoadCredentials(); err == nil {
		t.Error("expected an error when no credentials are stored")
	}
}
