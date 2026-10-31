package wolfbet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok-123", "hash-456")
	return c
}

func assertErrorIs(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("got error %v, want %v", got, want)
	}
}

// ---- client_test.go: auth headers + GET endpoints ----

func TestAuthHeadersSent(t *testing.T) {
	var gotAuth, gotHash, gotClient, gotRequested, gotCT string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotHash = r.Header.Get("X-Hash-Api")
		gotClient = r.Header.Get("X-Client-Type")
		gotRequested = r.Header.Get("X-Requested-With")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})
	_, _, _ = c.do(context.Background(), http.MethodGet, "/api/v2/user/balances", nil)
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotHash != "hash-456" {
		t.Errorf("X-Hash-Api = %q", gotHash)
	}
	if gotClient != "Web-Application" {
		t.Errorf("X-Client-Type = %q", gotClient)
	}
	if gotRequested != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q", gotRequested)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q", gotCT)
	}
}

func TestBalancesParses(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/user/balances" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"userBalances":[{"currency":"trx","amount":"12.5"},{"currency":"ltc","amount":0.001}]}`))
	})
	bal, err := c.Balances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bal) != 2 {
		t.Fatalf("len = %d, want 2", len(bal))
	}
	if bal[0].Currency != "trx" || bal[0].Amount.Float() != 12.5 {
		t.Errorf("bal[0] = %+v", bal[0])
	}
	amount, ok := BalanceFor(bal, "TRX")
	if !ok || amount != 12.5 {
		t.Errorf("BalanceFor(TRX) = %v %v", amount, ok)
	}
	if _, ok := BalanceFor(bal, "btc"); ok {
		t.Error("BalanceFor(btc) should miss")
	}
}

func TestCurrenciesParses(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/site/currencies" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"settings":{"trx":{"casino":{"amount_minimum":0.0000001,"amount_maximum":100}}}}`))
	})
	cur, err := c.Currencies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	set, ok := cur["trx"]
	if !ok {
		t.Fatalf("settings keys = %v", cur)
	}
	if set.AmountMinimum.Float() != 0.0000001 || set.AmountMaximum.Float() != 100 {
		t.Errorf("trx settings = %+v", set)
	}
}

func TestRefreshSeed(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/game/seed/refresh" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	if err := c.RefreshSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBalancesAuthError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	})
	_, err := c.Balances(context.Background())
	assertErrorIs(t, err, ErrAuth)
}

// ---- dice_test.go: payload math + error mapping ----

func TestBuildDicePayloadUnder(t *testing.T) {
	p := BuildDicePayload(DiceBet{Amount: 0.000001, Chance: 49.5, Bethigh: false, Currency: "TRX"})
	if p.Amount != "0.00000100" {
		t.Errorf("amount = %q, want 0.00000100", p.Amount)
	}
	if p.BetValue != 49.5 {
		t.Errorf("bet_value = %v, want 49.5", p.BetValue)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("multiplier = %v, want 2", p.Multiplier)
	}
	if p.Rule != "under" {
		t.Errorf("rule = %q", p.Rule)
	}
	if p.Currency != "trx" {
		t.Errorf("currency = %q", p.Currency)
	}
	if p.Game != "dice" {
		t.Errorf("game = %q", p.Game)
	}
	if p.Auto != 1 {
		t.Errorf("auto = %d", p.Auto)
	}
}

func TestBuildDicePayloadOver(t *testing.T) {
	p := BuildDicePayload(DiceBet{Amount: 1, Chance: 49.5, Bethigh: true, Currency: "ltc"})
	if p.BetValue != 50.49 {
		t.Errorf("bet_value = %v, want 50.49 (99.99-49.5)", p.BetValue)
	}
	if p.Rule != "over" {
		t.Errorf("rule = %q", p.Rule)
	}
}

func TestBuildDicePayloadRounding(t *testing.T) {
	// chance 33.333 rounds to 33.33; multiplier 99/33.33 = 2.9703 (4dp)
	p := BuildDicePayload(DiceBet{Amount: 2, Chance: 33.333, Bethigh: false, Currency: "trx"})
	if p.BetValue != 33.33 {
		t.Errorf("bet_value = %v, want 33.33", p.BetValue)
	}
	if p.Multiplier != 2.9703 {
		t.Errorf("multiplier = %v, want 2.9703", p.Multiplier)
	}
}

func TestPlaceDiceBetHappyPath(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/bet/place" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["amount"] != "0.00000100" {
			t.Errorf("amount = %v", body["amount"])
		}
		if body["game"] != "dice" || body["auto"] != float64(1) {
			t.Errorf("body = %v", body)
		}
		w.Header().Set("x-ratelimit-remaining", "87")
		_, _ = w.Write([]byte(`{"bet":{"profit":0.000001,"result_value":12.34,"nonce":5,"hash":"abc123"}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{
		Amount: 0.000001, Chance: 49.5, Bethigh: false, Currency: "trx",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll != 12.34 {
		t.Errorf("roll = %v", res.Roll)
	}
	if res.Profit != 0.000001 {
		t.Errorf("profit = %v", res.Profit)
	}
	if res.Nonce != 5 {
		t.Errorf("nonce = %d", res.Nonce)
	}
	if res.ID != "abc123" {
		t.Errorf("id = %q", res.ID)
	}
	// under 49.5: roll 12.34 < 49.5 → win
	if !res.Won {
		t.Error("won = false, want true (12.34 < 49.5)")
	}
	if res.RateLimitRemaining != 87 {
		t.Errorf("rate limit remaining = %d, want 87", res.RateLimitRemaining)
	}
}

func TestPlaceDiceBetWinOver(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"bet":{"profit":1,"result_value":60,"nonce":1,"hash":"h"}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: true, Currency: "trx"})
	if err != nil {
		t.Fatal(err)
	}
	// over 50.49: roll 60 > 50.49 → win
	if !res.Won {
		t.Error("won = false, want true (60 > 50.49)")
	}
}

func TestPlaceDiceBetLoss(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"bet":{"profit":-1,"result_value":50,"nonce":1,"hash":"h"}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	if err != nil {
		t.Fatal(err)
	}
	// under 49.5: roll 50 >= 49.5 → loss
	if res.Won {
		t.Error("won = true, want false (50 >= 49.5)")
	}
}

func TestPlaceDiceBetOneBetAtATime(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"You can play only one bet at a time."}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	assertErrorIs(t, err, ErrBusy)
}

func TestPlaceDiceBetOtherBodyErrorStops(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"Game is under maintenance"}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want APIError", err)
	}
	if apiErr.Message != "Game is under maintenance" {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestPlaceDiceBetRateLimited(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	assertErrorIs(t, err, ErrRateLimited)
}

func TestPlaceDiceBetInsufficient(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	assertErrorIs(t, err, ErrInsufficient)
}

func TestPlaceDiceBetValidation(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"amount":["The amount field is required."],"bet_value":["The bet value field must be a number."]}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("got %v, want ValidationError", err)
	}
	if len(vErr.Fields) != 2 {
		t.Errorf("fields = %v", vErr.Fields)
	}
	if !strings.Contains(vErr.Error(), "amount: ") {
		t.Errorf("error should log field: message, got %q", vErr.Error())
	}
}

func TestPlaceDiceBetAuth(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	assertErrorIs(t, err, ErrAuth)
	if !strings.Contains(ErrAuth.Error(), "token") {
		t.Error("ErrAuth should explain how to refresh token/user_hash")
	}
}

func TestPlaceDiceBetNetworkError(t *testing.T) {
	// a server that is closed before the call → request-level failure
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := New(srv.URL, "t", "h")
	srv.Close()
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	assertErrorIs(t, err, ErrNetwork)
}

func TestPlaceDiceBetDefaultRateLimitHeader(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"bet":{"profit":0,"result_value":1,"nonce":0,"hash":"h"}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RateLimitRemaining != 120 {
		t.Errorf("rate limit default = %d, want 120", res.RateLimitRemaining)
	}
}
