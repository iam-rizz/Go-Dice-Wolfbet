package lua

import "regexp"

// Compatibility replace chain, ported verbatim from
// apps/bot/src/engine/lua-bridge.ts (original lua-mode start(), line ~2113).
// Order matters: `!=`→`~=` FIRST, then `!`→`not `, then the compound
// assignment expansions.
var (
	reNotEqual = regexp.MustCompile(`!=`)
	reBang     = regexp.MustCompile(`!`)
	rePlusEq   = regexp.MustCompile(`([a-zA-Z]*[0-9]*\s*)\+\=(\s*[a-zA-Z]*[0-9]*)`)
	reMinusEq  = regexp.MustCompile(`([a-zA-Z]*[0-9]*\s*)\-\=(\s*[a-zA-Z]*[0-9]*)`)
	reStarEq   = regexp.MustCompile(`([a-zA-Z]*[0-9]*\s*)\*\=(\s*[a-zA-Z]*[0-9]*)`)
	reSlashEq  = regexp.MustCompile(`([a-zA-Z]*[0-9]*\s*)\/\=(\s*[a-zA-Z]*[0-9]*)`)
)

// ApplyCompatReplacements rewrites legacy script syntax that the original
// engine tolerated before loading the script into the VM.
func ApplyCompatReplacements(source string) string {
	out := reNotEqual.ReplaceAllString(source, `~=`)
	out = reBang.ReplaceAllString(out, `not `)
	out = rePlusEq.ReplaceAllString(out, `$1=$1+$2 `)
	out = reMinusEq.ReplaceAllString(out, `$1=$1-$2 `)
	out = reStarEq.ReplaceAllString(out, `$1=$1*$2 `)
	out = reSlashEq.ReplaceAllString(out, `$1=$1/$2 `)
	return out
}
