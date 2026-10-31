package state

import (
	"regexp"
	"strconv"
	"strings"
)

// LastBet mirrors the dual-case Lua contract: every field exists in both
// lowercase and Capitalized spelling. Values are stored once here; the
// dual spelling is produced at the Lua boundary (ExposeToLua) and read
// back at collect-returns time — both spellings are always identical.
type LastBet struct {
	Amount  float64
	Chance  float64
	Roll    float64
	Profit  float64
	Target  float64
	Chance1 float64
	Chance2 float64
	Chance3 float64
	Chance4 float64
	BetSide string
	Result  bool
	Nonce   int64
	// ID is number or string in the original contract (hash vs numeric id).
	ID any
}

// CurrencyRates mirrors `currencyRates: { usd: { rate } }`. Nil = rate 1.
type CurrencyRates struct {
	USD struct {
		Rate float64
	}
}

// BotState is the full 40+ variable contract exposed to user scripts.
// Field names follow the original camelCase Lua names.
type BotState struct {
	Win            bool
	Bets           int
	Wins           int
	Losses         int
	Winstreak      int
	Losestreak     int
	Currentstreak  int
	Balance        float64
	Minbalance     float64
	Maxbalance     float64
	Minprofit      float64
	Maxprofit      float64
	MaxBetAmount   float64
	MaxLosseAmount float64
	Maxwinstreak   int
	Maxlosestreak  int
	Chance         float64
	Bethigh        bool
	Chance1        float64
	Chance2        float64
	Chance3        float64
	Chance4        float64
	BetSide        string
	Nextbet        float64
	Previousbet    float64
	Profit         float64
	Currentprofit  float64
	Partialprofit  float64
	Wagered        float64
	Currency       string
	Result         bool
	Target         float64
	Target1        float64
	Target2        float64
	Target3        float64
	Target4        float64
	Minbet         float64
	Maxbet         float64
	StartBalance   float64
	PercentWagered float64
	PercentProfit  float64

	LastBet LastBet

	// game-specific optionals
	PlinkoRisk  string
	PlinkoRow   int
	KenoRisk    string
	KenoNumbers []int
	MinesPicks  []int
	MinesTarget int

	// runtime
	Mode          string
	Game          string
	Run           bool
	Coin          string
	CurrencyRates *CurrencyRates
}

// BetRoll is the roll payload returned by the casino API.
type BetRoll struct {
	Roll       float64
	RollResult bool
	WonValue   float64
	BetValue   float64
	ID         any
	Nonce      *int64
}

func lastBetDefaults() LastBet {
	return LastBet{
		BetSide: "between",
	}
}

// CreateInitialState returns the original contract defaults.
func CreateInitialState() *BotState {
	return &BotState{
		Bethigh:    true,
		BetSide:    "between",
		Currency:   "",
		PlinkoRisk: "low",
		KenoRisk:   "classic",
		Mode:       "lua",
		Game:       "dice",
		LastBet:    lastBetDefaults(),
	}
}

// luaNumber formats a float the way JavaScript's String(v) does, because
// the original engine built Lua literals from JS values. Go's %g prints
// "2e-08" where JS prints "2e-8"; both are valid Lua, but we normalize
// to keep output byte-compatible with the original format.
var luaExpZero = regexp.MustCompile(`e([+-])0(\d)`)

func luaNumber(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	s = luaExpZero.ReplaceAllString(s, "e${1}${2}")
	// JS prints 1e+15 as "1000000000000000"; Go 'g' keeps the exponent.
	// Values that large never occur in balances/bets, so %g is fine here.
	return s
}

func luaValue(v any) string {
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case string:
		return `"` + val + `"`
	case float64:
		return luaNumber(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case nil:
		return "nil"
	case []int:
		parts := make([]string, len(val))
		for i, n := range val {
			parts[i] = luaNumber(float64(n))
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return "nil"
	}
}

type luaField struct {
	key string
	val any
}

var lastBetFields = func(lb LastBet) []luaField {
	return []luaField{
		{"amount", lb.Amount},
		{"Amount", lb.Amount},
		{"chance", lb.Chance},
		{"Chance", lb.Chance},
		{"roll", lb.Roll},
		{"Roll", lb.Roll},
		{"profit", lb.Profit},
		{"Profit", lb.Profit},
		{"target", lb.Target},
		{"Target", lb.Target},
		{"chance1", lb.Chance1},
		{"Chance1", lb.Chance1},
		{"chance2", lb.Chance2},
		{"Chance2", lb.Chance2},
		{"chance3", lb.Chance3},
		{"Chance3", lb.Chance3},
		{"chance4", lb.Chance4},
		{"Chance4", lb.Chance4},
		{"betSide", lb.BetSide},
		{"BetSide", lb.BetSide},
		{"result", lb.Result},
		{"Result", lb.Result},
		{"nonce", lb.Nonce},
		{"Nonce", lb.Nonce},
		{"id", lb.ID},
		{"Id", lb.ID},
	}
}

// ExposeToLua generates the Lua declarations assigning every state
// variable, run before each dobet() call. Pattern matches the original:
// `win=false\nbets=0\n...lastBet={\n amount=...,\n ...}`.
func ExposeToLua(s *BotState) string {
	scalars := []luaField{
		{"win", s.Win},
		{"bets", s.Bets},
		{"wins", s.Wins},
		{"losses", s.Losses},
		{"winstreak", s.Winstreak},
		{"losestreak", s.Losestreak},
		{"currentstreak", s.Currentstreak},
		{"maxwinstreak", s.Maxwinstreak},
		{"maxlosestreak", s.Maxlosestreak},
		{"balance", s.Balance},
		{"minbalance", s.Minbalance},
		{"maxbalance", s.Maxbalance},
		{"minprofit", s.Minprofit},
		{"maxprofit", s.Maxprofit},
		{"chance", s.Chance},
		{"bethigh", s.Bethigh},
		{"chance1", s.Chance1},
		{"chance2", s.Chance2},
		{"chance3", s.Chance3},
		{"chance4", s.Chance4},
		{"betSide", s.BetSide},
		{"nextbet", s.Nextbet},
		{"previousbet", s.Previousbet},
		{"profit", s.Profit},
		{"currentprofit", s.Currentprofit},
		{"partialprofit", s.Partialprofit},
		{"wagered", s.Wagered},
		{"currency", s.Currency},
		{"result", s.Result},
		{"target", s.Target},
		{"target1", s.Target1},
		{"target2", s.Target2},
		{"target3", s.Target3},
		{"target4", s.Target4},
		{"minbet", s.Minbet},
		{"maxbet", s.Maxbet},
		{"startBalance", s.StartBalance},
		{"percentWagered", s.PercentWagered},
		{"percentProfit", s.PercentProfit},
		{"plinkoRisk", s.PlinkoRisk},
		{"plinkoRow", s.PlinkoRow},
		{"kenoRisk", s.KenoRisk},
		{"kenoNumbers", sliceOrNil(s.KenoNumbers)},
		{"minesPicks", sliceOrNil(s.MinesPicks)},
		{"minesTarget", s.MinesTarget},
	}

	lines := make([]string, 0, len(scalars)+1)
	for _, f := range scalars {
		lines = append(lines, f.key+"="+luaValue(f.val))
	}

	lb := lastBetFields(s.LastBet)
	parts := make([]string, len(lb))
	for i, f := range lb {
		parts[i] = f.key + "=" + luaValue(f.val)
	}
	lines = append(lines, "lastBet={\n "+strings.Join(parts, ",\n ")+"\n}")
	return strings.Join(lines, "\n") + "\n"
}

// sliceOrNil keeps the "empty slice = nil" contract: expose emits `nil`
// when the original (nullable) field is unset.
func sliceOrNil(s []int) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

// ApplyBetResult mirrors the original handleResult + handleStats logic.
// Mutates state (the engine loop owns the object) and returns it.
func ApplyBetResult(s *BotState, roll BetRoll) *BotState {
	rate := 1.0
	if s.CurrencyRates != nil {
		rate = s.CurrencyRates.USD.Rate
	}

	s.Result = roll.RollResult
	s.Win = roll.RollResult

	s.LastBet.Roll = roll.Roll
	s.LastBet.ID = roll.ID
	if roll.Nonce != nil {
		s.LastBet.Nonce = *roll.Nonce
	}
	s.LastBet.Chance = s.Chance
	s.LastBet.BetSide = s.BetSide
	s.LastBet.Target = s.Target
	s.LastBet.Result = roll.RollResult

	s.Currentprofit = (roll.WonValue - roll.BetValue) * rate
	s.LastBet.Profit = s.Currentprofit
	s.Profit += s.Currentprofit

	s.Wagered += s.LastBet.Amount
	s.Previousbet = s.LastBet.Amount

	s.Bets++
	if s.Win {
		s.Wins++
		s.Winstreak++
		s.Losestreak = 0
		s.Currentstreak = s.Winstreak
	} else {
		s.Losses++
		s.Losestreak++
		s.Winstreak = 0
		s.Currentstreak = -s.Losestreak
		if s.Previousbet > s.MaxLosseAmount {
			s.MaxLosseAmount = s.Previousbet
		}
	}
	if s.LastBet.Amount > s.MaxBetAmount {
		s.MaxBetAmount = s.LastBet.Amount
	}
	if s.Winstreak > s.Maxwinstreak {
		s.Maxwinstreak = s.Winstreak
	}
	if s.Losestreak > s.Maxlosestreak {
		s.Maxlosestreak = s.Losestreak
	}

	s.Balance += s.Currentprofit
	if s.Bets == 1 || s.Balance < s.Minbalance {
		s.Minbalance = s.Balance
	}
	if s.Balance > s.Maxbalance {
		s.Maxbalance = s.Balance
	}
	if s.Bets == 1 || s.Profit < s.Minprofit {
		s.Minprofit = s.Profit
	}
	if s.Profit > s.Maxprofit {
		s.Maxprofit = s.Profit
	}

	// snapshot for the next bet (original sets lastBet.amount to nextbet)
	s.LastBet.Amount = s.Nextbet

	if s.StartBalance != 0 {
		s.PercentProfit = (s.Profit / s.StartBalance) * 100
		s.PercentWagered = (s.Wagered / s.StartBalance) * 100
	}

	return s
}
