package lua

import "wdb-go/internal/state"

// ApplyReturns pushes the collected script variables back into the
// engine state — the original playBet/updateLua write-back. Nil fields
// ("script never set it") leave the engine state untouched.
func ApplyReturns(st *state.BotState, ret Returns) {
	if ret.Nextbet != nil {
		st.Nextbet = *ret.Nextbet
	}
	if ret.Chance != nil {
		st.Chance = *ret.Chance
	}
	if ret.Bethigh != nil {
		st.Bethigh = *ret.Bethigh
	}
	if ret.Chance1 != nil {
		st.Chance1 = *ret.Chance1
	}
	if ret.Chance2 != nil {
		st.Chance2 = *ret.Chance2
	}
	if ret.Chance3 != nil {
		st.Chance3 = *ret.Chance3
	}
	if ret.Chance4 != nil {
		st.Chance4 = *ret.Chance4
	}
	if ret.BetSide != nil {
		st.BetSide = *ret.BetSide
	}
	if ret.PlinkoRisk != nil {
		st.PlinkoRisk = *ret.PlinkoRisk
	}
	if ret.PlinkoRow != nil {
		st.PlinkoRow = *ret.PlinkoRow
	}
	if ret.KenoRisk != nil {
		st.KenoRisk = *ret.KenoRisk
	}
	if ret.KenoNumbers != nil {
		st.KenoNumbers = ret.KenoNumbers
	}
	if ret.MinesPicks != nil {
		st.MinesPicks = ret.MinesPicks
	}
	if ret.MinesTarget != nil {
		st.MinesTarget = *ret.MinesTarget
	}
	if ret.Currency != nil {
		st.Currency = *ret.Currency
	}
	if ret.Target != nil {
		st.Target = *ret.Target
	}
}
