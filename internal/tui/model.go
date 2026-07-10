// Package tui is the Bubble Tea client for a Big 2 session: one Model per
// connection, rendering the room's per-viewer snapshots and submitting actions.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Avangelista/deuception/internal/game"
	"github.com/Avangelista/deuception/internal/protocol"
	"github.com/Avangelista/deuception/internal/room"
)

// commander is the subset of *room.Room the TUI needs (submit actions).
type commander interface {
	Submit(room.Command)
}

type quitMsg struct{}

// Model is a single connection's view state.
type Model struct {
	room     commander
	id       string
	joinHint string // "ssh -p PORT IP" shown in the waiting room
	prog     *tea.Program

	r  *lipgloss.Renderer
	st styles

	w, h int
	snap *protocol.StateSnapshot

	cursor   int
	scroll   int // off-turn view offset (leftmost visible card); no cursor is shown
	selected map[int]bool
	hint     string
	hintGen  int  // bumped on each hint so a stale timer can't clear a newer hint
	lastRev  int  // highest snapshot revision applied; drops out-of-order deliveries
	boss     bool // "boss key": blank the card borders so the board looks like plain text
	kicked   string
}

type hintExpireMsg struct{ gen int }

// New builds a Model; renderer must be session-scoped (MakeRenderer for SSH).
func New(r commander, id, joinHint string, renderer *lipgloss.Renderer) *Model {
	return &Model{
		room:     r,
		id:       id,
		joinHint: joinHint,
		r:        renderer,
		st:       newStyles(renderer),
		selected: map[int]bool{},
	}
}

// SetProgram records the program so the room can push updates.
func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

func (m *Model) Init() tea.Cmd { return nil }

// Update handles input and pushed room messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampScroll() // resize changed how many cards fit; keep scroll in range
	case protocol.StateSnapshotMsg:
		m.applySnapshot(msg.Snap)
	case protocol.ErrorMsg:
		return m, m.setHint(msg.Text)
	case hintExpireMsg:
		if msg.gen == m.hintGen {
			m.hint = ""
		}
	case protocol.KickedMsg:
		m.kicked = msg.Reason
		return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return quitMsg{} })
	case protocol.RoomClosedMsg:
		m.kicked = "room closed"
		return m, tea.Quit
	case quitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) applySnapshot(s protocol.StateSnapshot) {
	// Snapshots arrive on their own goroutines, so a later fanout can land first;
	// ignore anything older than the newest applied.
	if s.Rev != 0 && s.Rev < m.lastRev {
		return
	}
	m.lastRev = s.Rev
	var prevHand []game.Card
	if m.snap != nil {
		prevHand = m.snap.YourHand
	}
	m.snap = &s
	// Reset selection/hint/scroll when the hand's contents change; keying on size
	// alone would miss an equal-size redeal and carry stale indices into it.
	if !sameHand(prevHand, s.YourHand) {
		m.selected = map[int]bool{}
		m.hint = ""
		m.scroll = 0
	}
	// Clear a stale "not your turn" once it's actually your turn.
	if s.Phase == protocol.InGame && s.Turn == s.YouSeat {
		m.hint = ""
	}
	if m.cursor >= len(s.YourHand) {
		m.cursor = max(0, len(s.YourHand)-1)
	}
	m.clampScroll()
}

// sameHand reports whether two hands hold the same cards in the same order.
func sameHand(a, b []game.Card) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clampScroll keeps the off-turn scroll within [0, len-maxHandCells] so a resize
// can't strand it past the end.
func (m *Model) clampScroll() {
	if m.snap == nil {
		return
	}
	maxScroll := len(m.snap.YourHand) - m.maxHandCells()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c", "q":
		m.room.Submit(room.QuitCmd{ID: m.id})
		return m, tea.Quit
	}
	if m.kicked != "" {
		return m, tea.Quit
	}
	if k.String() == "b" {
		m.boss = !m.boss // boss key: toggle the plain-text disguise
		return m, nil
	}
	if m.snap == nil {
		return m, nil
	}
	switch m.snap.Phase {
	case protocol.Waiting:
		return m.keyWaiting(k)
	case protocol.InGame:
		return m.keyGame(k)
	case protocol.Finished:
		return m.keyOver(k)
	}
	return m, nil
}

func (m *Model) keyWaiting(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "s":
		if m.snap.IsHost {
			m.room.Submit(room.StartCmd{ID: m.id})
		}
	}
	return m, nil
}

func (m *Model) keyGame(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	hand := m.snap.YourHand
	myTurn := m.isMyTurn()
	switch k.String() {
	case "left":
		// On turn move the cursor; off turn scroll the view (no cursor).
		if myTurn {
			if m.cursor > 0 {
				m.cursor--
			}
		} else if m.scroll > 0 {
			m.scroll--
		}
	case "right":
		if myTurn {
			if m.cursor < len(hand)-1 {
				m.cursor++
			}
		} else if m.scroll < len(hand)-m.maxHandCells() {
			m.scroll++
		}
	case " ":
		if !myTurn || len(hand) == 0 {
			return m, nil
		}
		switch {
		case m.selected[m.cursor]:
			delete(m.selected, m.cursor)
			m.hint = ""
		case len(m.selected) < 5:
			m.selected[m.cursor] = true // combos are at most 5 cards
			m.hint = ""
		default:
			return m, m.setHint("select up to 5 cards")
		}
	case "c":
		m.selected = map[int]bool{}
		m.hint = ""
	case "enter":
		if !myTurn {
			return m, nil
		}
		cards := m.selectedCards()
		if len(cards) == 0 {
			return m, nil
		}
		m.room.Submit(room.PlayCmd{ID: m.id, Cards: cards})
	case "x":
		if !myTurn {
			return m, nil
		}
		m.selected = map[int]bool{} // passing discards any pending selection
		m.room.Submit(room.PassCmd{ID: m.id})
	}
	return m, nil
}

// isMyTurn reports whether the game is live and it is this viewer's turn.
func (m *Model) isMyTurn() bool {
	return m.snap != nil && m.snap.Phase == protocol.InGame && m.snap.Turn == m.snap.YouSeat
}

// setHint shows a transient hint, cleared after a few seconds unless a newer one
// replaces it first.
func (m *Model) setHint(text string) tea.Cmd {
	m.hint = text
	m.hintGen++
	gen := m.hintGen
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return hintExpireMsg{gen} })
}

func (m *Model) keyOver(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "n":
		if m.snap.IsHost {
			m.room.Submit(room.NextHandCmd{ID: m.id})
		}
	}
	return m, nil
}

func (m *Model) selectedCards() []game.Card {
	hand := m.snap.YourHand
	out := make([]game.Card, 0, len(m.selected))
	for i := 0; i < len(hand); i++ {
		if m.selected[i] {
			out = append(out, hand[i])
		}
	}
	return out
}

// View renders the current screen, applying the boss-key disguise last.
func (m *Model) View() string {
	out := m.viewContent()
	if m.boss {
		out = bossHide(out)
	}
	return out
}

func (m *Model) viewContent() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		return m.tooSmall()
	}
	if m.kicked != "" {
		return m.renderKicked()
	}
	if m.snap == nil {
		return m.center("connecting...")
	}
	switch m.snap.Phase {
	case protocol.Waiting:
		return m.renderWaiting()
	case protocol.InGame:
		return m.renderGame()
	case protocol.Finished:
		return m.renderOver()
	}
	return ""
}

func (m *Model) center(s string) string {
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, s)
}
