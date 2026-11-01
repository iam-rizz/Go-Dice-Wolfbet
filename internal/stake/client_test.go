package stake

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
	return New(srv.URL, "sess-abc")
}

func assertErrorIs(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("got error %v, want %v", got, want)
	}
}

// ---- auth + envelope ----

func TestGraphQLPathAndAuthHeaders(t *testing.T) {
	var gotPath, gotToken, gotCT string
	var gotBody graphqlRequest
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("x-access-token")
		gotCT = r.Header.Get("content-type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{}}`))
	})

	_, err := c.Balances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/_api/graphql" {
		t.Errorf("path = %q, want /_api/graphql", gotPath)
	}
	if gotToken != "sess-abc" {
		t.Errorf("x-access-token = %q", gotToken)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotBody.OperationName != "UserBalances" {
		t.Errorf("operationName = %q", gotBody.OperationName)
	}
}

func TestBalancesParses(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"user":{"balances":[
			{"available":{"amount":12.5,"currency":"trx"}},
			{"available":{"amount":0.001,"currency":"ltc"}}
		]}}}`))
	})
	bal, err := c.Balances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bal) != 2 {
		t.Fatalf("got %d balances, want 2", len(bal))
	}
	if v, ok := BalanceFor(bal, "trx"); !ok || v != 12.5 {
		t.Errorf("trx = %v (ok=%v), want 12.5", v, ok)
	}
	if _, ok := BalanceFor(bal, "doge"); ok {
		t.Error("doge should not exist")
	}
}

func TestGraphQLErrorsSurfaceAsAPIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	})
	_, err := c.Balances(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want *APIError", err)
	}
	if apiErr.Message != "boom" {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestUnauthorizedStops(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.Balances(context.Background())
	assertErrorIs(t, err, ErrAuth)
}

// ---- DiceRoll ----

func TestDiceRollPayloadAndMapping(t *testing.T) {
	var gotBody graphqlRequest
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"diceRoll":{
			"id":"12345","nonce":7,"amount":1.0,"payout":1.98,
			"state":{"result":55.12}
		}}}`))
	})

	res, err := c.PlaceDiceBet(context.Background(), DiceBet{
		Amount: 1.0, Chance: 49.5, Bethigh: true, Currency: "TRX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "12345" || res.Nonce != 7 {
		t.Errorf("id/nonce = %q/%d", res.ID, res.Nonce)
	}
	if res.Profit != 0.98 {
		t.Errorf("profit = %v, want 0.98 (payout-amount)", res.Profit)
	}
	if res.Roll != 55.12 {
		t.Errorf("roll = %v, want 55.12", res.Roll)
	}

	// payload: target/condition derived from chance + bethigh
	vars := gotBody.Variables
	if vars["condition"] != "above" {
		t.Errorf("condition = %v, want above (bethigh)", vars["condition"])
	}
	if vars["target"] != 50.5 {
		t.Errorf("target = %v, want 50.5 (100-49.5)", vars["target"])
	}
	if vars["currency"] != "trx" {
		t.Errorf("currency = %v, want lowercase trx", vars["currency"])
	}
	if id, _ := vars["identifier"].(string); len(id) != 20 {
		t.Errorf("identifier = %q, want 20 chars", id)
	}
	if gotBody.OperationName != "DiceRoll" {
		t.Errorf("operationName = %q", gotBody.OperationName)
	}
}

func TestDiceRollBelowCondition(t *testing.T) {
	var gotBody graphqlRequest
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"diceRoll":{"id":"1","nonce":1,"amount":1,"payout":0,"state":{"result":80}}}}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 22, Bethigh: false, Currency: "doge"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.Variables["condition"] != "below" {
		t.Errorf("condition = %v, want below", gotBody.Variables["condition"])
	}
	if gotBody.Variables["target"] != 22.0 {
		t.Errorf("target = %v, want 22", gotBody.Variables["target"])
	}
}

func TestDiceRollSlowDownRetries(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"errors":[{"message":"Please slow down"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"diceRoll":{"id":"9","nonce":2,"amount":1,"payout":2,"state":{"result":10}}}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 50, Bethigh: false, Currency: "trx"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (retry after slow down)", calls)
	}
	if res.ID != "9" {
		t.Errorf("id = %q", res.ID)
	}
}

func TestDiceRollSeedRotateRetries(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.OperationName == "RotateSeedPair" {
			_, _ = w.Write([]byte(`{"data":{"rotateSeedPair":{}}}`))
			return
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"errors":[{"message":"an unknown error 294779 has happened."}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"diceRoll":{"id":"5","nonce":3,"amount":1,"payout":1,"state":{"result":42}}}}`))
	})
	res, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 50, Bethigh: false, Currency: "trx"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "5" {
		t.Errorf("id = %q, want 5 after seed rotation", res.ID)
	}
}

func TestDiceRollUnknownErrorStops(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"insufficient funds"}]}`))
	})
	_, err := c.PlaceDiceBet(context.Background(), DiceBet{Amount: 1, Chance: 50, Bethigh: false, Currency: "trx"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want *APIError", err)
	}
}

func TestRotateSeedPairSendsMutation(t *testing.T) {
	var gotBody graphqlRequest
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"rotateSeedPair":{}}}`))
	})
	if err := c.RotateSeedPair(context.Background(), "seed123"); err != nil {
		t.Fatal(err)
	}
	if gotBody.OperationName != "RotateSeedPair" {
		t.Errorf("operationName = %q", gotBody.OperationName)
	}
	if gotBody.Variables["seed"] != "seed123" {
		t.Errorf("seed = %v", gotBody.Variables["seed"])
	}
}
