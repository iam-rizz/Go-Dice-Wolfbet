// Package tui renders the live session view: stats column, script log
// ring, and key handling (q quit, s stop-on-win toggle).
package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wdb-go/internal/state"
)

// LogRingSize is the retained log line count (spec: last 200 lines).
const LogRingSize = 200

// Messages pushed into the TUI by the engine goroutine.
type (
	// StatsMsg carries a state snapshot after each bet.
	StatsMsg struct {
		State     *state.BotState
		RateLimit int
		Speed     float64 // bets per second
	}
	// LogMsg is one script log()/system line.
	LogMsg string
	// StopMsg marks the loop stopped (reason: max-bets, error, stop-on-win...).
	StopMsg struct{ Reason string }
	// ErrorMsg surfaces a fatal error.
	ErrorMsg struct{ Err string }
	// StoppedMsg tells the model the user asked to quit.
	QuitMsg struct{}
)

// Status values for the footer/header.
const (
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusError   = "error"
)

// Model is the bubbletea model. It never touches the engine directly:
// everything arrives as messages, and user intents leave via callbacks.
type Model struct {
	ScriptName string
	Mode       string // "live" | "dry-run"
	Currency   string

	Status     string
	StopReason string
	Err        string

	// stats snapshot
	Balance       float64
	Profit        float64
	Bets          int
	Wins          int
	Losses        int
	Winstreak     int
	Losestreak    int
	MaxWinstreak  int
	MaxLosestreak int
	Currentstreak int
	Wagered       float64
	Chance        float64
	CurrentBet    float64
	RateLimit     int
	Speed         float64

	StopOnWin bool

	logs []string

	// OnStopOnWinToggle fires when the user presses 's'.
	OnStopOnWinToggle func(stopOnWin bool)
	// OnQuit fires when the user presses 'q' / ctrl+c.
	OnQuit func()
}

// New builds the model with initial state values.
func New(scriptName, mode, currency string, stopOnWin bool) Model {
	return Model{
		ScriptName: scriptName,
		Mode:       mode,
		Currency:   currency,
		Status:     StatusRunning,
		StopOnWin:  stopOnWin,
		logs:       make([]string, 0, LogRingSize),
	}
}

// PushLog appends a line to the ring, trimming to the last LogRingSize.
func (m *Model) PushLog(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > LogRingSize {
		m.logs = m.logs[len(m.logs)-LogRingSize:]
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles keys and engine messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			if m.OnQuit != nil {
				m.OnQuit()
			}
			return m, tea.Quit
		case "s":
			m.StopOnWin = !m.StopOnWin
			if m.OnStopOnWinToggle != nil {
				m.OnStopOnWinToggle(m.StopOnWin)
			}
			return m, nil
		}
	case StatsMsg:
		s := msg.State
		m.Balance = s.Balance
		m.Profit = s.Profit
		m.Bets = s.Bets
		m.Wins = s.Wins
		m.Losses = s.Losses
		m.Winstreak = s.Winstreak
		m.Losestreak = s.Losestreak
		m.MaxWinstreak = s.Maxwinstreak
		m.MaxLosestreak = s.Maxlosestreak
		m.Currentstreak = s.Currentstreak
		m.Wagered = s.Wagered
		m.Chance = s.Chance
		m.CurrentBet = s.Nextbet
		m.RateLimit = msg.RateLimit
		m.Speed = msg.Speed
		m.Status = StatusRunning
		return m, nil
	case LogMsg:
		m.PushLog(string(msg))
		return m, nil
	case StopMsg:
		m.Status = StatusStopped
		m.StopReason = msg.Reason
		return m, nil
	case ErrorMsg:
		m.Status = StatusError
		m.Err = msg.Err
		return m, nil
	case QuitMsg:
		return m, tea.Quit
	}
	return m, nil
}

var (
	styleBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	styleTitle      = lipgloss.NewStyle().Bold(true)
	styleProfitUp   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleProfitDown = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	styleFooter     = lipgloss.NewStyle().Faint(true)
)

// View renders the two-column layout with a footer.
func (m Model) View() string {
	left := m.statsView()
	right := m.logView()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := styleFooter.Render(m.footerView())
	return strings.Join([]string{body, "", footer}, "\n")
}

func (m Model) statsView() string {
	profit := formatFloat(m.Profit)
	if m.Profit > 0 {
		profit = styleProfitUp.Render("+" + profit)
	} else if m.Profit < 0 {
		profit = styleProfitDown.Render(profit)
	}
	streak := formatInt(m.Winstreak) + "/" + formatInt(m.Losestreak)
	maxStreak := formatInt(m.MaxWinstreak) + "/" + formatInt(m.MaxLosestreak)

	var winrate string
	if m.Bets > 0 {
		winrate = strconv.FormatFloat(float64(m.Wins)/float64(m.Bets)*100, 'f', 1, 64) + "%"
	} else {
		winrate = "-"
	}

	lines := []string{
		styleTitle.Render("xWDice — " + m.ScriptName),
		"mode:     " + m.Mode,
		"currency: " + m.Currency,
		"",
		"balance:  " + formatFloat(m.Balance),
		"profit:   " + profit,
		"wagered:  " + formatFloat(m.Wagered),
		"",
		"chance:   " + formatFloat(m.Chance),
		"bets:     " + formatInt(m.Bets),
		"wins:     " + formatInt(m.Wins),
		"losses:   " + formatInt(m.Losses),
		"winrate:  " + winrate,
		"streak:   " + streak,
		"max strk: " + maxStreak,
		"current:  " + formatFloat(m.CurrentBet),
		"",
		"speed:    " + formatFloat(m.Speed) + " bets/s",
		"rate:     " + formatInt(m.RateLimit) + " remaining",
	}
	if m.Status != StatusRunning {
		lines = append(lines, "", "status:   "+m.Status+statusSuffix(m))
	}
	return styleBorder.Render(strings.Join(lines, "\n"))
}

func statusSuffix(m Model) string {
	switch m.Status {
	case StatusStopped:
		if m.StopReason != "" {
			return " (" + m.StopReason + ")"
		}
	case StatusError:
		if m.Err != "" {
			return " (" + m.Err + ")"
		}
	}
	return ""
}

func (m Model) logView() string {
	const visible = 20
	start := 0
	if len(m.logs) > visible {
		start = len(m.logs) - visible
	}
	shown := m.logs[start:]
	if len(shown) == 0 {
		shown = []string{"(no log output yet)"}
	}
	return styleBorder.Render(strings.Join(shown, "\n"))
}

func (m Model) footerView() string {
	stopWin := "off"
	if m.StopOnWin {
		stopWin = "on"
	}
	return m.Mode + " · " + m.Currency + " · stop-on-win: " + stopWin +
		" · [q] quit · [s] toggle stop-on-win"
}

func formatFloat(v float64) string {
	// 8 decimals for coin amounts, trailing zeros trimmed
	s := strconv.FormatFloat(v, 'f', 8, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func formatInt(v int) string {
	return strconv.Itoa(v)
}
