// Package login automates the stake.com browser login so the dice bot can
// obtain the `session` cookie without manual DevTools work.
//
// Stake is behind Cloudflare Turnstile, so a plain HTTP login is impossible
// — this drives a real Chromium instance (chromedp) which passes the
// challenge, fills the form, handles the 2FA prompt, and extracts the
// resulting session cookie.
package login

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Credentials holds the stake login material. Password is stored locally
// (0600) so repeat logins need no typing; OTP is asked interactively when
// the account has 2FA.
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Dir is where credentials and the cached session live.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".wdbgo")
}

func credentialsPath() string { return filepath.Join(Dir(), "credentials") }
func sessionPath() string     { return filepath.Join(Dir(), "session") }

// SaveCredentials writes the login material with owner-only permissions.
func SaveCredentials(c Credentials) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(credentialsPath(), data, 0o600)
}

// LoadCredentials reads the stored login material.
func LoadCredentials() (Credentials, error) {
	data, err := os.ReadFile(credentialsPath())
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

// SaveSession caches the session cookie so repeat runs skip the browser.
func SaveSession(session string) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), []byte(session), 0o600)
}

// LoadSession returns the cached session cookie, or "" when absent.
func LoadSession() string {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ErrNoCredentials is returned when no login material is stored yet.
var ErrNoCredentials = errors.New("no credentials stored — run: wdbgo-login --setup")

// promptLine reads one line from stdin with the given prompt.
func promptLine(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// EnsureCredentials returns stored credentials, prompting for any missing
// field interactively. The result is persisted for next time.
func EnsureCredentials() (Credentials, error) {
	c, err := LoadCredentials()
	if err != nil {
		c = Credentials{}
	}
	changed := false
	if c.Email == "" {
		email, err := promptLine("Stake email: ")
		if err != nil {
			return Credentials{}, err
		}
		c.Email = email
		changed = true
	}
	if c.Password == "" {
		password, err := promptLine("Stake password: ")
		if err != nil {
			return Credentials{}, err
		}
		c.Password = password
		changed = true
	}
	if changed {
		if err := SaveCredentials(c); err != nil {
			return Credentials{}, err
		}
		fmt.Println("✓ kredensial disimpan di", credentialsPath())
	}
	return c, nil
}

// promptOTP asks for the 2FA code when stake requires one.
func promptOTP() (string, error) {
	code, err := promptLine("Kode 2FA/OTP (kosongkan jika tidak diminta): ")
	if err != nil {
		return "", err
	}
	return code, nil
}

// now is a seam for tests.
var now = time.Now
