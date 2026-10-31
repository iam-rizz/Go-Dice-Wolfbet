package engine

import "context"

// BetRequest is one bet the engine wants placed.
type BetRequest struct {
	Amount   float64
	Chance   float64
	Bethigh  bool
	Currency string
}

// BetOutcome is the mapped casino response.
type BetOutcome struct {
	Roll               float64
	Profit             float64
	Won                bool
	Target             float64
	Nonce              int64
	ID                 string
	Amount             float64
	RateLimitRemaining int
}

// BetPlacer places a bet — implemented by the wolfbet client (live) and
// the dry-run simulator. Dry-run is a drop-in replacement: the loop never
// knows which one it holds.
type BetPlacer interface {
	PlaceBet(ctx context.Context, req BetRequest) (BetOutcome, error)
}

// Script is the user-script runner the loop drives: one dobet() per bet
// (after the first), with the script-set variables written back into
// state.
type Script interface {
	// RunDobet injects state, runs dobet() and applies the collected
	// script variables (nextbet, chance, bethigh...) back into state.
	RunDobet() error
}

// ScriptSetup runs once after script load: the top-level assignments
// (nextbet/chance/bethigh) land in state BEFORE bet 1 — bet 1 uses them
// without calling dobet (original sequencing, engine index.ts:676).
type ScriptSetup interface {
	CollectTopLevel() error
}
