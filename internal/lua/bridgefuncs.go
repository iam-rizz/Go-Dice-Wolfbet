package lua

import (
	"strconv"
	"time"

	glua "github.com/yuin/gopher-lua"
)

// BridgeFunctionNames is the 16-function contract, in the original
// injection order.
var BridgeFunctionNames = []string{
	"start",
	"stop",
	"resume",
	"checkbalance",
	"resetstats",
	"resetseed",
	"resetchart",
	"resethistory",
	"resetlog",
	"resetall",
	"ching",
	"log",
	"invest",
	"vault",
	"unvault",
	"resetpartialprofit",
}

// sleepMaxSeconds caps sleep() like the original bridge (safety cap the
// original lacked; a bad n must not pin the bot forever).
const sleepMaxSeconds = 10

// injectBridgeFunctions registers every contract function as a plain Lua
// global backed by the host callbacks. The original used js.global:X()
// wrappers because of fengari interop; gopher-lua calls Go closures
// directly, which is behaviorally identical for the script.
func (b *Bridge) injectBridgeFunctions() {
	for _, name := range BridgeFunctionNames {
		name := name
		b.L.SetGlobal(name, b.L.NewFunction(func(L *glua.LState) int {
			cb, ok := b.callbacks[name]
			if !ok {
				return 0
			}
			args := make([]any, 0, 2)
			for i := 1; i <= 2; i++ {
				v := L.Get(i)
				if v == glua.LNil {
					args = append(args, nil)
					continue
				}
				switch lv := v.(type) {
				case glua.LNumber:
					args = append(args, float64(lv))
				case glua.LString:
					args = append(args, string(lv))
				case glua.LBool:
					args = append(args, bool(lv))
				default:
					args = append(args, nil)
				}
			}
			ret := cb(args...)
			switch val := ret.(type) {
			case nil:
				return 0
			case float64:
				L.Push(glua.LNumber(val))
				return 1
			case string:
				L.Push(glua.LString(val))
				return 1
			case bool:
				L.Push(glua.LBool(val))
				return 1
			default:
				return 0
			}
		}))
	}
}

// injectPrimitives ports the remaining initLua globals:
//   - clock = os.clock
//   - sleep(n): a REAL sleep (the wolf bundle uses a Promise sleep, not
//     the 500-casino busy-wait) — time.Sleep with the 10s safety cap
//   - casino="<hostname>"
//
// table.getn is native in Lua 5.1 (gopher-lua), so no shim is needed.
func (b *Bridge) injectPrimitives() {
	_ = b.L.DoString(`clock = os.clock`)

	b.L.SetGlobal("sleep", b.L.NewFunction(func(L *glua.LState) int {
		v := L.Get(1)
		var secs float64
		switch lv := v.(type) {
		case glua.LNumber:
			secs = float64(lv)
		case glua.LString:
			s, err := strconv.ParseFloat(string(lv), 64)
			if err == nil {
				secs = s
			}
		}
		if secs <= 0 {
			return 0
		}
		if secs > sleepMaxSeconds {
			secs = sleepMaxSeconds
		}
		time.Sleep(time.Duration(secs * float64(time.Second)))
		return 0
	}))

	b.L.SetGlobal("casino", glua.LString(b.casino))
	b.L.SetGlobal("scriptname", glua.LString(""))
}
