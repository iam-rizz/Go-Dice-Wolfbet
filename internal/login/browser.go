package login

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// LoginResult reports what happened during a browser login.
type LoginResult struct {
	Session string
	Email   string
}

const (
	loginURL      = "https://stake.com/login"
	challengeWait = 60 * time.Second // Cloudflare "Just a moment" can be slow
	formWait      = 30 * time.Second
	otpWait       = 120 * time.Second // user needs time to fetch the code
)

// candidateSelectors are tried in order for each field — stake's form
// markup changes, so we accept several shapes.
var (
	emailSelectors = []string{
		`input[name="email"]`,
		`input[type="email"]`,
		`input[placeholder*="mail" i]`,
		`input[id*="email" i]`,
	}
	passwordSelectors = []string{
		`input[name="password"]`,
		`input[type="password"]`,
		`input[placeholder*="assword" i]`,
	}
	otpSelectors = []string{
		`input[name="code"]`,
		`input[autocomplete="one-time-code"]`,
		`input[inputmode="numeric"]`,
		`input[placeholder*="ode" i]`,
		`input[id*="code" i]`,
		`input[id*="otp" i]`,
	}
	submitSelectors = []string{
		`button[type="submit"]`,
		`button:has-text("Log In")`,
		`button:has-text("Sign In")`,
		`button:has-text("Login")`,
	}
)

// firstVisibleSelector returns the first selector matching a visible node.
func firstVisibleSelector(ctx context.Context, selectors []string) (string, error) {
	for _, sel := range selectors {
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](
			`(() => { const el = document.querySelector(`+jsString(sel)+`); return !!el && el.offsetParent !== null; })()`,
		))
		if err == nil && ok {
			return sel, nil
		}
	}
	return "", fmt.Errorf("no visible field found (tried %s)", strings.Join(selectors, ", "))
}

// jsString quotes a Go string as a JS string literal.
func jsString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// waitChallenge polls until the Cloudflare "Just a moment" title is gone.
func waitChallenge(ctx context.Context) error {
	deadline := now().Add(challengeWait)
	for now().Before(deadline) {
		title, err := chromedp.Run(ctx, chromedp.Title())
		if err != nil {
			return err
		}
		if !strings.Contains(strings.ToLower(title), "just a moment") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("cloudflare challenge not solved within %s", challengeWait)
}

// waitSessionCookie polls the browser cookies until `session` appears.
func waitSessionCookie(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := now().Add(timeout)
	for now().Before(deadline) {
		session, err := chromedp.Run(ctx, chromedp.Evaluate[string](
			`(() => { const m = document.cookie.match(/(?:^|;\s*)session=([^;]+)/); return m ? m[1] : ""; })()`,
		))
		if err == nil && session != "" {
			return session, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("session cookie did not appear within %s", timeout)
}

// Login drives Chromium through the stake login flow and returns the
// session cookie. OTP is requested interactively only when the 2FA field
// shows up.
func Login(ctx context.Context, creds Credentials) (*LoginResult, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true), // required when running as root
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1280,900"),
		// look less like a bot
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	if err := chromedp.Do(taskCtx, chromedp.Navigate(loginURL)); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Cloudflare challenge — a real browser solves it automatically.
	fmt.Println("… menunggu Cloudflare challenge")
	if err := waitChallenge(taskCtx); err != nil {
		return nil, err
	}

	// email
	emailSel, err := firstVisibleSelector(taskCtx, emailSelectors)
	if err != nil {
		return nil, fmt.Errorf("email field: %w", err)
	}
	if err := chromedp.Do(taskCtx,
		chromedp.WaitVisible(emailSel),
		chromedp.SendKeys(emailSel, creds.Email),
	); err != nil {
		return nil, fmt.Errorf("fill email: %w", err)
	}

	// password
	passSel, err := firstVisibleSelector(taskCtx, passwordSelectors)
	if err != nil {
		return nil, fmt.Errorf("password field: %w", err)
	}
	if err := chromedp.Do(taskCtx,
		chromedp.WaitVisible(passSel),
		chromedp.SendKeys(passSel, creds.Password),
	); err != nil {
		return nil, fmt.Errorf("fill password: %w", err)
	}

	// submit
	submitSel, err := firstVisibleSelector(taskCtx, submitSelectors)
	if err != nil {
		// fall back to pressing Enter in the password field
		if err := chromedp.Do(taskCtx, chromedp.KeyEvent("\r")); err != nil {
			return nil, fmt.Errorf("submit: %w", err)
		}
	} else {
		if err := chromedp.Do(taskCtx, chromedp.Click(submitSel)); err != nil {
			return nil, fmt.Errorf("click submit: %w", err)
		}
	}

	fmt.Println("… mengirim kredensial")

	// 2FA: if a code field appears, ask the user.
	otpSel, otpErr := firstVisibleSelector(taskCtx, otpSelectors)
	if otpErr == nil {
		fmt.Println("… stake meminta kode 2FA")
		code, err := promptOTP()
		if err != nil {
			return nil, err
		}
		if code != "" {
			if err := chromedp.Do(taskCtx,
				chromedp.WaitVisible(otpSel),
				chromedp.SendKeys(otpSel, code),
			); err != nil {
				return nil, fmt.Errorf("fill otp: %w", err)
			}
			// submit the OTP form too
			if subSel, err := firstVisibleSelector(taskCtx, submitSelectors); err == nil {
				_ = chromedp.Do(taskCtx, chromedp.Click(subSel))
			} else {
				_ = chromedp.Do(taskCtx, chromedp.KeyEvent("\r"))
			}
		}
	}

	fmt.Println("… menunggu session cookie")
	session, err := waitSessionCookie(taskCtx, otpWait)
	if err != nil {
		// capture the page state for debugging
		title, _ := chromedp.Run(taskCtx, chromedp.Title())
		body, _ := chromedp.Run(taskCtx, chromedp.Evaluate[string](`document.body.innerText.slice(0,500)`))
		return nil, fmt.Errorf("%w (page: %q / %q)", err, title, body)
	}

	return &LoginResult{Session: session, Email: creds.Email}, nil
}
