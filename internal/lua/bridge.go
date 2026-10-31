package lua

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	glua "github.com/yuin/gopher-lua"

	"wdb-go/internal/state"
)

// LuaScriptError mirrors the original error type: load-phase vs runtime-phase.
type LuaScriptError struct {
	Message string
	Phase   string // "load" | "runtime"
}

func (e *LuaScriptError) Error() string {
	return fmt.Sprintf("lua %s error: %s", e.Phase, e.Message)
}

var ErrNoDobet = errors.New("script must define function dobet()")

// Callbacks host the 16 bridge function implementations. A missing entry
// is a no-op (matches the original: unimplemented actions just do nothing).
type Callbacks map[string]func(args ...any) any

// Bridge is the gopher-lua counterpart of the original fengari LuaBridge:
// one VM per script, state injected before every dobet(), script-writable
// variables collected after.
type Bridge struct {
	L         *glua.LState
	state     *state.BotState
	callbacks Callbacks
	casino    string
}

// New creates the VM, injects the 16 bridge functions and the initLua
// primitives (clock, sleep, table.getn, casino).
func New(st *state.BotState, callbacks Callbacks, casino string) *Bridge {
	b := &Bridge{
		L:         glua.NewState(),
		state:     st,
		callbacks: callbacks,
		casino:    casino,
	}
	b.L.OpenLibs()
	b.injectBridgeFunctions()
	b.injectPrimitives()
	return b
}

// Close releases the VM.
func (b *Bridge) Close() {
	b.L.Close()
}

// LoadUserScript applies the compat chain and runs the script. The script
// must define `function dobet()`. Mirrors the original start(): reset
// scriptname + keno globals, transform, then execute.
func (b *Bridge) LoadUserScript(script string) error {
	if err := b.L.DoString(`scriptname = ""`); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "load"}
	}
	if err := b.L.DoString(`kenoNumbers = nil; kenoRisk = nil`); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "load"}
	}
	transformed := ApplyCompatReplacements(script)
	if err := b.L.DoString(transformed); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "load"}
	}
	dobet := b.L.GetGlobal("dobet")
	if dobet == glua.LNil || dobet.Type() != glua.LTFunction {
		return &LuaScriptError{Message: ErrNoDobet.Error(), Phase: "load"}
	}
	return nil
}

// InjectState writes every state variable as Lua globals (original
// updateLua pattern).
func (b *Bridge) InjectState() error {
	if err := b.L.DoString(state.ExposeToLua(b.state)); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "runtime"}
	}
	cur := strconv.Quote(b.state.Currency)
	if err := b.L.DoString("currency=" + cur); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "runtime"}
	}
	return nil
}

// RunDobet is one betting iteration: inject state, then call dobet().
// Lua runtime errors are returned as *LuaScriptError — the loop stops.
func (b *Bridge) RunDobet() error {
	if err := b.InjectState(); err != nil {
		return err
	}
	if err := b.L.DoString(`dobet()`); err != nil {
		return &LuaScriptError{Message: err.Error(), Phase: "runtime"}
	}
	return nil
}

// Returns holds the script-writable variables collected after dobet().
// A nil field means "script never set it — engine keeps its own state".
type Returns struct {
	Nextbet     *float64
	Chance      *float64
	Bethigh     *bool
	Chance1     *float64
	Chance2     *float64
	Chance3     *float64
	Chance4     *float64
	BetSide     *string
	PlinkoRisk  *string
	PlinkoRow   *int
	KenoRisk    *string
	KenoNumbers []int // nil = skip
	MinesPicks  []int // nil = skip
	MinesTarget *int
	Currency    *string
	Target      *float64
}

// num resolves a global with the original numeric coercion: numbers pass
// through; non-empty numeric strings are parsed; everything else is nil.
func (b *Bridge) num(name string) *float64 {
	v := b.L.GetGlobal(name)
	switch lv := v.(type) {
	case glua.LNumber:
		f := float64(lv)
		return &f
	case glua.LString:
		s := string(lv)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func (b *Bridge) str(name string) *string {
	v := b.L.GetGlobal(name)
	if lstr, ok := v.(glua.LString); ok && string(lstr) != "" {
		s := string(lstr)
		return &s
	}
	return nil
}

func (b *Bridge) boolean(name string) *bool {
	v := b.L.GetGlobal(name)
	if lb, ok := v.(glua.LBool); ok {
		bv := bool(lb)
		return &bv
	}
	return nil
}

// intPtr collects an integer-typed state variable (plinkoRow, minesTarget).
func (b *Bridge) intPtr(name string) *int {
	f := b.num(name)
	if f == nil {
		return nil
	}
	i := int(*f)
	return &i
}

// tableToNumbers reads a Lua table global as a numeric array, like the
// original luaTableToArray (numbers or numeric strings, index 1..n).
// Returns nil when the global is not a table.
func (b *Bridge) tableToNumbers(name string) []int {
	v := b.L.GetGlobal(name)
	tbl, ok := v.(*glua.LTable)
	if !ok {
		return nil
	}
	length := tbl.Len()
	if length == 0 {
		// empty table still counts as "set" in the original
		return []int{}
	}
	out := make([]int, 0, length)
	for i := 1; i <= length; i++ {
		item := tbl.RawGetInt(i)
		var f float64
		switch lv := item.(type) {
		case glua.LNumber:
			f = float64(lv)
		case glua.LString:
			var err error
			f, err = strconv.ParseFloat(strings.TrimSpace(string(lv)), 64)
			if err != nil {
				return nil
			}
		default:
			return nil
		}
		out = append(out, int(f))
	}
	return out
}

// CollectReturns reads the script-writable variables after dobet().
func (b *Bridge) CollectReturns() Returns {
	var out Returns
	out.Nextbet = b.num("nextbet")
	out.Chance = b.num("chance")
	out.Bethigh = b.boolean("bethigh")
	out.Chance1 = b.num("chance1")
	out.Chance2 = b.num("chance2")
	out.Chance3 = b.num("chance3")
	out.Chance4 = b.num("chance4")
	out.BetSide = b.str("betSide")
	out.PlinkoRisk = b.str("plinkoRisk")
	out.PlinkoRow = b.intPtr("plinkoRow")
	out.KenoRisk = b.str("kenoRisk")
	out.KenoNumbers = b.tableToNumbers("kenoNumbers")
	out.MinesPicks = b.tableToNumbers("minesPicks")
	out.MinesTarget = b.intPtr("minesTarget")
	out.Currency = b.str("currency")
	out.Target = b.num("target")
	return out
}

// GetScriptName resolves the user script's name global.
func (b *Bridge) GetScriptName() string {
	v := b.L.GetGlobal("scriptname")
	if lstr, ok := v.(glua.LString); ok {
		return string(lstr)
	}
	return ""
}
