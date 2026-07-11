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
	boss     bool // hide the card UI (blank the borders so the board reads as plain text)
	kicked   string

	pendingBotLevel int // difficulty applied to the next added bot (1-9)

	pileCur  []game.Card // the play currently shown in the pile
	pilePrev []game.Card // the play it beat, drawn under the slide (same size within a trick)
	pileDir  [2]int      // unit direction the current play slides in from
	pileStep int         // slide frame, 0 (at the side) .. pileSteps (centred/at rest)
	pileGen  int         // invalidates stale slide ticks
}

type hintExpireMsg struct{ gen int }

// New builds a Model; renderer must be session-scoped (MakeRenderer for SSH).
func New(r commander, id, joinHint string, renderer *lipgloss.Renderer) *Model {
	return &Model{
		room:            r,
		id:              id,
		joinHint:        joinHint,
		r:               renderer,
		st:              newStyles(renderer),
		selected:        map[int]bool{},
		pendingBotLevel: 5,
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
		return m, m.applySnapshot(msg.Snap)
	case pileAnimMsg:
		return m, m.advancePile(msg)
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

func (m *Model) applySnapshot(s protocol.StateSnapshot) tea.Cmd {
	// Snapshots arrive on their own goroutines, so a later fanout can land first;
	// ignore anything older than the newest applied.
	if s.Rev != 0 && s.Rev < m.lastRev {
		return nil
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
	cmd := m.updatePile(s)
	if m.cursor >= len(s.YourHand) {
		m.cursor = max(0, len(s.YourHand)-1)
	}
	m.clampScroll()
	return cmd
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

// pileAnimMsg advances the play-in slide one frame; gen drops ticks from a slide
// that has already been superseded by a newer play.
type pileAnimMsg struct{ gen int }

// pileSteps and pileTickEvery time the play-in slide: a short glide from the
// player's side to the centre (pileSteps frames, ~pileSteps*pileTickEvery total).
const (
	pileSteps     = 8
	pileTickEvery = 22 * time.Millisecond
)

// updatePile reacts to a snapshot: a new table combo starts a slide from the side
// of the player who made it, opaquely covering the play it beat (both are the same
// size within a trick). An empty table or a non-playing phase clears the pile.
func (m *Model) updatePile(s protocol.StateSnapshot) tea.Cmd {
	if s.Phase != protocol.InGame || len(s.Table) == 0 {
		m.pileCur, m.pilePrev, m.pileDir, m.pileStep = nil, nil, [2]int{}, 0
		return nil
	}
	if sameHand(m.pileCur, s.Table) {
		return nil // same play still on the table
	}
	prev := m.pileCur
	m.pileCur = append([]game.Card(nil), s.Table...)
	// Cover the beaten play only when it is the same size (guaranteed within a
	// trick). A size change means the trick reset without us seeing the empty-table
	// snapshot, so there is nothing to cover.
	m.pilePrev = nil
	if len(prev) == len(m.pileCur) {
		m.pilePrev = prev
	}
	dx, dy := 0, 0
	if s.TableBy >= 0 {
		n := len(s.Players)
		dx, dy = pileNudge((s.TableBy-s.YouSeat+n)%n, n)
	}
	m.pileDir = [2]int{dx, dy}
	m.pileGen++
	if dx == 0 && dy == 0 { // no direction: just show it centred
		m.pileStep, m.pilePrev = pileSteps, nil
		return nil
	}
	m.pileStep = 0
	return m.pileTick()
}

// pileTick schedules the next slide frame, tagged with the current generation.
func (m *Model) pileTick() tea.Cmd {
	gen := m.pileGen
	return tea.Tick(pileTickEvery, func(time.Time) tea.Msg { return pileAnimMsg{gen: gen} })
}

// advancePile steps the slide, dropping the covered play once it settles centred.
func (m *Model) advancePile(msg pileAnimMsg) tea.Cmd {
	if msg.gen != m.pileGen || m.pileStep >= pileSteps {
		return nil
	}
	m.pileStep++
	if m.pileStep >= pileSteps {
		m.pilePrev = nil // fully covered now: only the current play remains
		return nil
	}
	return m.pileTick()
}

// SettlePile fast-forwards any in-flight slide to its resting centred frame. Used by
// the headless preview and tests, which don't run the tick loop.
func (m *Model) SettlePile() {
	m.pileStep, m.pilePrev = pileSteps, nil
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
	case "ctrl+c", "esc":
		m.room.Submit(room.QuitCmd{ID: m.id})
		return m, tea.Quit
	}
	if m.kicked != "" {
		return m, tea.Quit
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
	key := k.String()
	switch {
	case key == "enter":
		if m.snap.IsHost {
			m.room.Submit(room.StartCmd{ID: m.id})
		}
	case key == "+" || key == "=": // '=' is the unshifted '+' key
		if m.snap.IsHost {
			m.room.Submit(room.AddBotCmd{ID: m.id, Level: m.pendingBotLevel})
		}
	case key == "-":
		if m.snap.IsHost {
			m.room.Submit(room.RemoveBotCmd{ID: m.id})
		}
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		if m.snap.IsHost {
			m.pendingBotLevel = int(key[0] - '0')
		}
	case len(key) == 1 && isLetter(key[0]):
		m.room.Submit(room.SetLetterCmd{ID: m.id, Letter: key[0]}) // server enforces uniqueness
	}
	return m, nil
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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
	case "h":
		m.boss = !m.boss // hide the card UI (in-game only)
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
	case "enter":
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
