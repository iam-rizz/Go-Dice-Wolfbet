package state

import (
	"regexp"
	"strings"
	"testing"
)

func stateWithBalance(t *testing.T, balance, rate float64) *BotState {
	t.Helper()
	s := CreateInitialState()
	s.Balance = balance
	s.StartBalance = balance
	s.Nextbet = 1
	s.LastBet.Amount = 1
	s.Chance = 49.5
	s.CurrencyRates = &CurrencyRates{}
	s.CurrencyRates.USD.Rate = rate
	return s
}

func TestCreateInitialState(t *testing.T) {
	s := CreateInitialState()
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"win", s.Win, false},
		{"bets", s.Bets, 0},
		{"wins", s.Wins, 0},
		{"losses", s.Losses, 0},
		{"bethigh", s.Bethigh, true},
		{"betSide", s.BetSide, "between"},
		{"plinkoRisk", s.PlinkoRisk, "low"},
		{"kenoRisk", s.KenoRisk, "classic"},
		{"mode", s.Mode, "lua"},
		{"game", s.Game, "dice"},
		{"run", s.Run, false},
		{"coin", s.Coin, ""},
		{"currencyRates", s.CurrencyRates == nil, true},
		{"kenoNumbers", s.KenoNumbers == nil, true},
		{"minesPicks", s.MinesPicks == nil, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestExposeToLuaDeclaresEveryContractVar(t *testing.T) {
	lua := ExposeToLua(CreateInitialState())
	vars := []string{
		"win", "bets", "wins", "losses", "winstreak", "losestreak", "currentstreak",
		"balance", "minbalance", "maxbalance", "minprofit", "maxprofit", "chance",
		"bethigh", "chance1", "chance2", "chance3", "chance4", "betSide", "nextbet",
		"previousbet", "profit", "currentprofit", "partialprofit", "wagered",
		"currency", "result", "target", "target1", "target2", "target3", "target4",
		"minbet", "maxbet", "startBalance", "percentWagered", "percentProfit",
		"plinkoRisk", "plinkoRow", "kenoRisk", "kenoNumbers", "minesPicks", "minesTarget",
		"lastBet",
	}
	for _, v := range vars {
		pat := regexp.MustCompile(`(^|\n)` + regexp.QuoteMeta(v) + `=`)
		if !pat.MatchString(lua) {
			t.Errorf("missing declaration for %s", v)
		}
	}
	if !strings.HasPrefix(lua, "win=false\n") {
		t.Errorf("lua should start with win=false\\n, got %q", lua[:20])
	}
}

func TestExposeToLuaLastBetDualCase(t *testing.T) {
	lua := ExposeToLua(CreateInitialState())
	pairs := [][2]string{
		{"amount", "Amount"}, {"chance", "Chance"}, {"roll", "Roll"},
		{"profit", "Profit"}, {"target", "Target"}, {"chance1", "Chance1"},
		{"chance2", "Chance2"}, {"chance3", "Chance3"}, {"chance4", "Chance4"},
		{"betSide", "BetSide"}, {"result", "Result"}, {"nonce", "Nonce"},
		{"id", "Id"},
	}
	for _, p := range pairs {
		if !strings.Contains(lua, "\n "+p[0]+"=") {
			t.Errorf("lastBet missing lowercase field %s", p[0])
		}
		if !strings.Contains(lua, "\n "+p[1]+"=") {
			t.Errorf("lastBet missing capitalized field %s", p[1])
		}
	}
	if !strings.Contains(lua, "lastBet={\n") {
		t.Error("lastBet table should open with lastBet={\\n")
	}
}

func TestExposeToLuaSerializesTypes(t *testing.T) {
	s := CreateInitialState()
	s.KenoNumbers = []int{1, 2, 3}
	lua := ExposeToLua(s)
	if !strings.Contains(lua, "kenoNumbers={1,2,3}") {
		t.Error("kenoNumbers should serialize as {1,2,3}")
	}
	if !strings.Contains(lua, "minesPicks=nil") {
		t.Error("unset minesPicks should serialize as nil")
	}
}

func TestApplyBetResultWin(t *testing.T) {
	s := stateWithBalance(t, 100, 1)
	ApplyBetResult(s, BetRoll{Roll: 88.5, RollResult: true, WonValue: 2, BetValue: 1, ID: 42})
	assert(t, "bets", s.Bets, 1)
	assert(t, "wins", s.Wins, 1)
	assert(t, "losses", s.Losses, 0)
	assert(t, "win", s.Win, true)
	assert(t, "result", s.Result, true)
	assertFloat(t, "currentprofit", s.Currentprofit, 1)
	assertFloat(t, "profit", s.Profit, 1)
	assertFloat(t, "balance", s.Balance, 101)
	assert(t, "winstreak", s.Winstreak, 1)
	assert(t, "currentstreak", s.Currentstreak, 1)
	assertFloat(t, "lastBet.roll", s.LastBet.Roll, 88.5)
	assert(t, "lastBet.id", s.LastBet.ID, 42)
	assertFloat(t, "lastBet.amount", s.LastBet.Amount, s.Nextbet)
}

func TestApplyBetResultLose(t *testing.T) {
	s := stateWithBalance(t, 100, 1)
	ApplyBetResult(s, BetRoll{Roll: 10.2, RollResult: false, WonValue: 0, BetValue: 1, ID: 43})
	assert(t, "bets", s.Bets, 1)
	assert(t, "losses", s.Losses, 1)
	assert(t, "wins", s.Wins, 0)
	assertFloat(t, "currentprofit", s.Currentprofit, -1)
	assertFloat(t, "profit", s.Profit, -1)
	assertFloat(t, "balance", s.Balance, 99)
	assert(t, "losestreak", s.Losestreak, 1)
	assert(t, "currentstreak", s.Currentstreak, -1)
	assertFloat(t, "minbalance", s.Minbalance, 99)
	assertFloat(t, "minprofit", s.Minprofit, -1)
	// initial max 0; first bet sets it to current balance
	assertFloat(t, "maxbalance", s.Maxbalance, 99)
}

func TestApplyBetResultMaximaAcrossBets(t *testing.T) {
	s := stateWithBalance(t, 100, 1)
	ApplyBetResult(s, BetRoll{Roll: 90, RollResult: true, WonValue: 3, BetValue: 1, ID: 1})
	assertFloat(t, "maxbalance", s.Maxbalance, 102)
	assertFloat(t, "maxprofit", s.Maxprofit, 2)
	ApplyBetResult(s, BetRoll{Roll: 5, RollResult: false, WonValue: 0, BetValue: 1, ID: 2})
	assertFloat(t, "profit", s.Profit, 1)
	assertFloat(t, "maxprofit", s.Maxprofit, 2)
	assertFloat(t, "maxbalance", s.Maxbalance, 102)
	assert(t, "currentstreak", s.Currentstreak, -1)
}

func TestApplyBetResultSessionMaxima(t *testing.T) {
	s := stateWithBalance(t, 100, 1)
	s.Nextbet = 0.00000002
	s.LastBet.Amount = 0.00000002
	ApplyBetResult(s, BetRoll{Roll: 90, RollResult: true, WonValue: 2, BetValue: 1, ID: 1})
	// lastBet.amount was set to nextbet (0.00000002) — becomes max bet
	assertFloat(t, "maxBetAmount", s.MaxBetAmount, 0.00000002)
	assert(t, "maxwinstreak", s.Maxwinstreak, 1)

	// a bigger bet then a loss: maxBetAmount grows, maxLosseAmount tracks the lost amount
	s.Nextbet = 0.00000005
	s.LastBet.Amount = 0.00000005
	ApplyBetResult(s, BetRoll{Roll: 10, RollResult: false, WonValue: 0, BetValue: 1, ID: 2})
	assertFloat(t, "maxBetAmount", s.MaxBetAmount, 0.00000005)
	assertFloat(t, "maxLosseAmount", s.MaxLosseAmount, 0.00000005)
	assert(t, "maxlosestreak", s.Maxlosestreak, 1)
}

func TestApplyBetResultScalesByRate(t *testing.T) {
	s := stateWithBalance(t, 100, 0.5)
	ApplyBetResult(s, BetRoll{Roll: 90, RollResult: true, WonValue: 2, BetValue: 1, ID: 9})
	assertFloat(t, "currentprofit", s.Currentprofit, 0.5)
}

func assert(t *testing.T, name string, got, want any) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	diff := got - want
	if diff < -1e-12 || diff > 1e-12 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}
