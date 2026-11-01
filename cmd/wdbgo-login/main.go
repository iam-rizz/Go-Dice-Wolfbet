// wdbgo-login — obtains the stake.com session cookie for wdbgo.
//
// Usage:
//
//	wdbgo-login              # automated browser login (needs an unblocked IP)
//	wdbgo-login --manual     # paste the session cookie from your browser
//	wdbgo-login --setup      # (re)enter email & password
//	wdbgo-login --status     # show whether a cached session exists
//	wdbgo-login --logout     # delete the cached session
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"wdb-go/internal/login"
)

func main() {
	setup := flag.Bool("setup", false, "prompt for and store email/password")
	manual := flag.Bool("manual", false, "paste the session cookie from your browser")
	status := flag.Bool("status", false, "show whether a cached session exists")
	logout := flag.Bool("logout", false, "delete the cached session")
	flag.Parse()

	switch {
	case *status:
		showStatus()
	case *logout:
		doLogout()
	case *manual:
		doManual()
	default:
		doLogin(*setup)
	}
}

func showStatus() {
	creds, err := login.LoadCredentials()
	if err != nil {
		fmt.Println("kredensial: belum ada (jalankan: wdbgo-login --setup)")
	} else {
		fmt.Println("kredensial: tersimpan untuk", creds.Email)
	}
	if s := login.LoadSession(); s != "" {
		fmt.Printf("session:    tersimpan (%d karakter) di %s\n", len(s), login.Dir()+"/session")
	} else {
		fmt.Println("session:    belum ada (jalankan: wdbgo-login --manual)")
	}
}

func doLogout() {
	if err := login.SaveSession(""); err != nil {
		fmt.Fprintln(os.Stderr, "wdbgo-login:", err)
		os.Exit(1)
	}
	fmt.Println("session dihapus.")
}

// doManual asks for the session cookie value and caches it. Accepts the
// raw cookie value or a full "session=..." pair.
func doManual() {
	fmt.Println("Tempel nilai cookie `session` dari browser Anda.")
	fmt.Println("(DevTools → Application → Cookies → https://stake.com → session)")
	fmt.Print("session: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(os.Stderr, "wdbgo-login: tidak ada input")
		os.Exit(1)
	}
	value := strings.TrimSpace(line)

	// accept "session=xxx" pasted wholesale
	if strings.HasPrefix(value, "session=") {
		value = strings.TrimPrefix(value, "session=")
	}
	// accept a full cookie header: "a=1; session=xxx; b=2"
	if strings.Contains(value, ";") {
		for _, part := range strings.Split(value, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "session=") {
				value = strings.TrimPrefix(part, "session=")
				break
			}
		}
	}
	if value == "" {
		fmt.Fprintln(os.Stderr, "wdbgo-login: session kosong")
		os.Exit(1)
	}

	if err := login.SaveSession(value); err != nil {
		fmt.Fprintln(os.Stderr, "wdbgo-login: gagal menyimpan:", err)
		os.Exit(1)
	}
	fmt.Println("✓ session disimpan di", login.Dir()+"/session")
	fmt.Println("  sekarang jalankan: ./wdbgo --casino stake --script <script.lua>")
}

func doLogin(setup bool) {
	creds, err := login.LoadCredentials()
	if err != nil || setup || creds.Email == "" || creds.Password == "" {
		creds, err = login.EnsureCredentials()
		if err != nil {
			fmt.Fprintln(os.Stderr, "wdbgo-login:", err)
			os.Exit(1)
		}
	}

	fmt.Println("membuka Chromium untuk login stake.com…")
	result, err := login.Login(context.Background(), creds)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wdbgo-login: login gagal:", err)
		os.Exit(1)
	}

	if err := login.SaveSession(result.Session); err != nil {
		fmt.Fprintln(os.Stderr, "wdbgo-login: gagal menyimpan session:", err)
		os.Exit(1)
	}
	fmt.Println("✓ login berhasil — session disimpan untuk", result.Email)
	fmt.Println("  sekarang jalankan: ./wdbgo --casino stake --script <script.lua>")
}
