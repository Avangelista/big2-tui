package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Avangelista/deuception/internal/game"
	"github.com/Avangelista/deuception/internal/protocol"
)

const (
	minW = 34
	minH = 13 // 4-player board: top band 2 + side fans >=5 + bottom (error + hand) 5
)

// ---- player letters & labels ----

// letterFor maps a seat to its fixed letter, the same for every viewer: A is the
// host (seat 0), then B, C, D.
func (m *Model) letterFor(seat int) byte {
	return byte('A' + seat)
}

func (m *Model) playerAtRel(rel int) protocol.PlayerView {
	n := len(m.snap.Players)
	return m.snap.Players[(m.snap.YouSeat+rel)%n]
}

// label renders a player as "<L> <count>": on turn in [brackets], otherwise
// space-padded to the same width so the layout never drifts as the turn moves.
func (m *Model) label(p protocol.PlayerView) string {
	inner := fmt.Sprintf("%c %d", m.letterFor(p.Seat), p.CardCount)
	if p.IsTurn {
		return m.st.turn.Render("[" + inner + "]")
	}
	return " " + inner + " "
}

// oppMark is a player's last-play marker pointing at the pile: arrow if they hold
// the current Table combo, "X" if they passed, else "". A hand active on its own
// turn is never marked.
func (m *Model) oppMark(p protocol.PlayerView, arrow string) string {
	if !p.Connected {
		return "D" // left the game
	}
	if p.IsYou && m.isMyTurn() {
		return ""
	}
	if m.snap.TableBy == p.Seat {
		return arrow
	}
	if p.Passed {
		return "X"
	}
	return ""
}

// bossReplacer blanks the card borders ("|" and "_") to spaces so columns stay
// aligned; bossHide runs it over a whole frame for the "boss key" disguise, which
// strips the box-drawing so the board reads as plain terminal text.
var bossReplacer = strings.NewReplacer("|", " ", "_", " ")

func bossHide(s string) string { return bossReplacer.Replace(s) }

// youHostTag labels a player as "(you, host)", "(you)", "(host)", or "". No
// leading space; the caller adds its own.
func youHostTag(p protocol.PlayerView) string {
	switch {
	case p.IsYou && p.IsHost:
		return "(you, host)"
	case p.IsYou:
		return "(you)"
	case p.IsHost:
		return "(host)"
	}
	return ""
}

// ---- game table (anchored to the screen edges: C top, B left, D right, A
// bottom, pile centre) ----

// tooSmall is the shared "enlarge your terminal" screen, shown once the window
// drops below the minimum.
func (m *Model) tooSmall() string {
	return m.center(fmt.Sprintf("enlarge terminal to %dx%d\n(now %dx%d)", minW, minH, m.w, m.h))
}

func (m *Model) renderGame() string {
	n := len(m.snap.Players)
	w, h := m.w, m.h

	// Bottom edge: an always-visible error line above the hand, centred over the
	// table.
	self := lipgloss.PlaceHorizontal(w, lipgloss.Center, m.selfBand())
	bottom := lipgloss.JoinVertical(lipgloss.Left,
		m.errorLine(w),
		self,
	)
	bottomH := lipgloss.Height(bottom)

	// Top edge: the opponent across the table (none in 3-player).
	top := m.topBand(n, w)
	topH := 0
	if top != "" {
		topH = lipgloss.Height(top)
	}

	midH := h - topH - bottomH
	if midH < 3 {
		midH = 3
	}

	parts := make([]string, 0, 3)
	if top != "" {
		parts = append(parts, lipgloss.PlaceHorizontal(w, lipgloss.Center, top))
	}
	parts = append(parts, m.midRow(n, w, midH))
	parts = append(parts, bottom)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// topBand: the across-the-table opponent's hidden hand, anchored to the top.
func (m *Model) topBand(n, w int) string {
	var p protocol.PlayerView
	switch n {
	case 4:
		p = m.playerAtRel(2)
	case 2:
		p = m.playerAtRel(1)
	default:
		return "" // 3 players: no top seat
	}
	top, bot := hFan(p.CardCount, w)
	ftop, fbot := m.st.faint.Render(top), m.st.faint.Render(bot)
	// The label rides row 0 and never moves (top and bot are the same width). The
	// band is a fixed 2 rows so the board never shifts: on turn the open top grows
	// down toward the centre, off turn a blank filler holds the second row.
	if p.IsTurn {
		// on turn: card grows into row 2, and an active player carries no marker.
		return lipgloss.JoinVertical(lipgloss.Left, ftop+m.label(p), fbot)
	}
	// Off turn the card is one line, so row 2 holds the last-play marker centred
	// under the cards: "v" played, "X" passed, blank otherwise.
	mark := lipgloss.PlaceHorizontal(lipgloss.Width(bot), lipgloss.Center, m.oppMark(p, "v"))
	return lipgloss.JoinVertical(lipgloss.Left, fbot+m.label(p), mark)
}

// midRow: left opponent flush-left, right opponent flush-right, pile centred,
// filling exactly midH rows.
func (m *Model) midRow(n, w, midH int) string {
	pile := m.pileBox()
	if n < 3 {
		return lipgloss.Place(w, midH, lipgloss.Center, lipgloss.Center, pile)
	}
	left := m.playerAtRel(1)      // B
	right := m.playerAtRel(n - 1) // D in 4p, C in 3p

	sideW := w / 4
	if sideW < 8 {
		sideW = 8
	}
	centerW := w - 2*sideW
	if centerW < 8 {
		centerW = 8
		sideW = (w - centerW) / 2
	}

	leftCol := lipgloss.Place(sideW, midH, lipgloss.Left, lipgloss.Center, m.sideBlock(left, midH-1, true))
	centerCol := lipgloss.Place(centerW, midH, lipgloss.Center, lipgloss.Center, pile)
	rightCol := lipgloss.Place(sideW, midH, lipgloss.Right, lipgloss.Center, m.sideBlock(right, midH-1, false))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, centerCol, rightCol)
}

// sideBlock: a side opponent's sideways fan, label pinned at the anchored outer
// edge (left player's big card at the bottom, right's at the top). On turn each
// card reaches toward the centre; off turn it recedes, so the label never moves.
func (m *Model) sideBlock(p protocol.PlayerView, budget int, leftSide bool) string {
	var fan []string
	align, arrow := lipgloss.Left, ">"
	if leftSide {
		fan = vFanLeft(p.CardCount, budget, p.IsTurn)
	} else {
		fan = vFanRight(p.CardCount, budget, p.IsTurn)
		align, arrow = lipgloss.Right, "<"
	}
	// Last-play marker on the centre-facing side, vertically centred: ">"/"<"
	// played, "X" passed.
	if mark := m.oppMark(p, arrow); mark != "" && len(fan) > 0 {
		mid := len(fan) / 2
		if leftSide {
			fan[mid] = fan[mid] + " " + mark
		} else {
			fan[mid] = mark + " " + fan[mid]
		}
	}
	return lipgloss.JoinVertical(align, append(fan, m.label(p))...)
}

// pileBox renders the current winning play as a horizontal face-up card box,
// empty on a new trick:
//
//	 __________
//	|4D|4H|2S  |
//	|  |  |    |
//	|__|__|____|
func (m *Model) pileBox() string {
	cs := m.snap.Table
	if len(cs) == 0 {
		return ""
	}
	var faces, blanks, bottom strings.Builder
	faces.WriteByte('|')
	blanks.WriteByte('|')
	bottom.WriteByte('|')
	for i, c := range cs {
		faces.WriteString(c.String())
		under, blank := "__", "  "
		if i == len(cs)-1 { // wider "big" front card, matching the hand
			faces.WriteString("  ")
			under, blank = "____", "    "
		}
		faces.WriteByte('|')
		blanks.WriteString(blank + "|")
		bottom.WriteString(under + "|")
	}
	w := lipgloss.Width(faces.String())
	return lipgloss.JoinVertical(lipgloss.Left,
		" "+strings.Repeat("_", w-2),
		faces.String(),
		blanks.String(),
		bottom.String(),
	)
}

// selfBand: the viewer's hand as a fanned row. A selected card lifts one row so
// its whole box clears the divider; an unselected one sits low, its bottom border
// off past the divider. The cursor card carries a "*" on its body. The rightmost
// is the "big" card; the label sits on the bottom row, and "<"/">" flag a scrolled
// window.
func (m *Model) selfBand() string {
	me := m.snap.Players[m.snap.YouSeat]
	hand := m.snap.YourHand
	label := m.label(me)
	if len(hand) == 0 {
		return "\n\n\n  " + label
	}
	myTurn := m.isMyTurn()
	maxCells := m.maxHandCells()

	var start, end int
	var moreLeft, moreRight bool
	if myTurn {
		// cursor-centred window that keeps the cursor in view
		start, end, moreLeft, moreRight = windowIndices(len(hand), m.cursor, maxCells)
	} else {
		// off turn: no cursor, scroll straight from m.scroll (the leftmost visible
		// card) so you can still look through the whole hand
		start = clampi(m.scroll, 0, maxi(0, len(hand)-maxCells))
		end = start + maxCells
		if end > len(hand) {
			end = len(hand)
		}
		moreLeft, moreRight = start > 0, end < len(hand)
	}

	rows := m.selfFan(hand, start, end, m.cursor, myTurn)
	if !myTurn {
		// Off turn the hand drops a row and sheds its cursor row; selfFan puts the
		// top border at [1] and faces at [2], with the last-play marker ("^" played,
		// "X" passed) riding just above.
		marker := ""
		if mk := m.oppMark(me, "^"); mk != "" {
			marker = lipgloss.PlaceHorizontal(len(rows[0]), lipgloss.Center, mk)
		}
		rows = []string{"", marker, rows[1], rows[2]}
	}
	// 2-col left margin keeps the fan aligned. The "<"/">" scroll flags ride on row
	// 2 either way (the face row on turn, a row above the dropped cards off turn).
	for r := range rows {
		rows[r] = "  " + rows[r]
	}
	if moreLeft {
		rows[2] = "< " + rows[2][2:]
	}
	if moreRight {
		rows[2] += " >"
	}
	rows[3] += label
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// maxHandCells is how many hand cards fit across the screen, reserving the 2-col
// left margin, box overhead and trailing label.
func (m *Model) maxHandCells() int {
	me := m.snap.Players[m.snap.YouSeat]
	n := (m.w - 6 - lipgloss.Width(m.label(me))) / 3
	if n < 1 {
		n = 1
	}
	return n
}

// selfFan renders the windowed hand as a fixed 4-row fan (top border, face, body,
// bottom). An unselected card sits low, its bottom border off-grid past the
// divider; a selected card lifts to row 0 so its whole box shows. The cursor card
// carries a "*" on its body row.
func (m *Model) selfFan(hand []game.Card, start, end, cursor int, showCursor bool) []string {
	count := end - start
	totalW := 3*(count-1) + 6 // last card sits at 3*(count-1), front cell is 6 wide
	rows := make([][]byte, 4)
	for r := range rows {
		rows[r] = []byte(strings.Repeat(" ", totalW))
	}
	put := func(r, c int, ch byte) {
		if r >= 0 && r < 4 && c >= 0 && c < totalW {
			rows[r][c] = ch
		}
	}
	for j := 0; j < count; j++ {
		i := start + j
		L := 3 * j
		faceW := 2
		if j == count-1 {
			faceW = 4 // the front "big" card
		}
		topRow := 1
		if m.selected[i] {
			topRow = 0 // selected: lifted up one row
		}
		faceRow, bodyRow, botRow := topRow+1, topRow+2, topRow+3
		// Left border runs down the card's visible body (put ignores botRow==4).
		put(faceRow, L, '|')
		put(bodyRow, L, '|')
		put(botRow, L, '|')
		// Top border, extended to bridge the diagonal down to a lower next card.
		topEnd := L + faceW
		if j < count-1 {
			nextTop := 1
			if m.selected[start+j+1] {
				nextTop = 0
			}
			switch {
			case nextTop > topRow: // next card is lower: roof slopes down to it
				topEnd = L + 4
			case nextTop == topRow: // same level: meet the next card's left edge
				topEnd = L + 3
			}
		}
		for c := L + 1; c <= topEnd; c++ {
			put(topRow, c, '_')
		}
		// Face (2 glyphs; the front card leaves its extra width blank).
		face := hand[i].String()
		put(faceRow, L+1, face[0])
		put(faceRow, L+2, face[1])
		// Cursor marker on the body, lower-left of the face.
		if showCursor && i == cursor {
			put(bodyRow, L+1, '*')
		}
		// Bottom border - only lands on-grid for a lifted (selected) card.
		for c := L + 1; c <= L+faceW; c++ {
			put(botRow, c, '_')
		}
		if j == count-1 {
			rb := L + faceW + 1
			put(faceRow, rb, '|')
			put(bodyRow, rb, '|')
			put(botRow, rb, '|')
		}
	}
	out := make([]string, 4)
	for r := range rows {
		out[r] = string(rows[r])
	}
	return out
}

// errorLine is the always-visible line above the hand for inline errors (e.g.
// "not your turn"): centred and wrapped to width, blank but present when there is
// no error.
func (m *Model) errorLine(w int) string {
	if m.hint == "" {
		return ""
	}
	return m.r.NewStyle().Width(w).Align(lipgloss.Center).Render(m.hint)
}

// ---- card-back fans (front card drawn larger, like a real fan) ----

// hFan draws the top opponent's horizontal fan: a wide front card then slivers,
// capped to what fits the width (minimum 3).
func hFan(count, w int) (string, string) {
	cap := (w - 12) / 3
	if cap < 3 {
		cap = 3
	}
	n := count
	if n > cap {
		n = cap
	}
	if n <= 0 {
		return "|", "|"
	}
	top := "|    " // front card (4 wide)
	bot := "|____"
	for i := 1; i < n; i++ {
		top += "|  " // sliver (2 wide)
		bot += "|__"
	}
	return top + "|", bot + "|"
}

// vFanLeft draws the left opponent's sideways fan, larger front card at the
// bottom, slivers showing the centre-facing right edge. active widens each card's
// body toward the centre; off turn it shrinks so the card recedes to the anchored
// left edge.
func vFanLeft(count, budget int, active bool) []string {
	body, gap := "_", " "
	if active {
		body, gap = "___", "   "
	}
	cards := clampi(count, 1, maxi(budget-2, 1))
	rows := make([]string, 0, cards+2)
	rows = append(rows, body)
	for i := 1; i < cards; i++ {
		rows = append(rows, body+"|")
	}
	rows = append(rows, gap+"|", body+"|") // larger front card at the bottom
	return rows
}

// vFanRight mirrors vFanLeft for the right opponent: larger front card at the top,
// slivers showing the centre-facing left edge, receding to the anchored right edge
// off turn.
func vFanRight(count, budget int, active bool) []string {
	body, gap := "_", " "
	if active {
		body, gap = "___", "   "
	}
	slivers := clampi(count-1, 0, maxi(budget-3, 0))
	rows := make([]string, 0, slivers+3)
	rows = append(rows, " "+body, "|"+gap, "|"+body) // larger front card at the top
	for i := 0; i < slivers; i++ {
		rows = append(rows, "|"+body)
	}
	return rows
}

// ---- waiting room ----

func (m *Model) renderWaiting() string {
	s := m.snap
	var b strings.Builder
	for i := 0; i < s.MaxSeats; i++ {
		if i < len(s.Players) {
			p := s.Players[i]
			line := string(m.letterFor(p.Seat))
			if t := youHostTag(p); t != "" {
				line += " " + t
			}
			if !p.Connected {
				line += " (gone)"
			}
			b.WriteString(line + "\n")
		} else {
			b.WriteString("-\n")
		}
	}
	b.WriteString("\n")
	ready := len(s.Players) >= s.MinStart
	switch {
	case s.IsHost && ready:
		b.WriteString("s: start\n")
	case s.IsHost:
		b.WriteString(fmt.Sprintf("need %d+ to start\n", s.MinStart))
	default:
		b.WriteString("waiting for host...\n")
	}
	if m.joinHint != "" {
		b.WriteString("\nothers join with:\n" + m.joinHint + "\n")
	}
	// compact two-column key legend to fit the minimum width
	for _, r := range [][4]string{
		{"arrows", "move", "x", "pass"},
		{"space", "select", "c", "clear"},
		{"enter", "play", "q", "quit"},
		{"b", "hide", "", ""},
	} {
		if r[2] == "" {
			b.WriteString(fmt.Sprintf("\n%-7s %s", r[0]+":", r[1]))
			continue
		}
		b.WriteString(fmt.Sprintf("\n%-7s %-6s  %-2s %s", r[0]+":", r[1], r[2]+":", r[3]))
	}
	return m.centerBlock(b.String())
}

// ---- game over ----

func (m *Model) renderOver() string {
	s := m.snap
	var b strings.Builder
	if s.Winner >= 0 {
		b.WriteString(m.st.turn.Render(fmt.Sprintf("%c wins", m.letterFor(s.Winner))) + "\n\n")
	}
	for _, p := range rankByScore(s.Players) {
		mark := ""
		if t := youHostTag(p); t != "" {
			mark = "  " + t
		}
		b.WriteString(fmt.Sprintf("%c %4d%s\n", m.letterFor(p.Seat), p.Score, mark))
	}
	b.WriteString("\n")
	if s.IsHost {
		b.WriteString("n: next hand")
	} else {
		b.WriteString("waiting for host...")
	}
	b.WriteString("\nq: quit")
	return m.centerBlock(b.String())
}

// centerBlock left-aligns a multi-line block into a rectangle and centres it on
// screen.
func (m *Model) centerBlock(content string) string {
	content = strings.TrimRight(content, "\n")
	block := m.r.NewStyle().Width(lipgloss.Width(content)).Align(lipgloss.Left).Render(content)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, block)
}

func rankByScore(players []protocol.PlayerView) []protocol.PlayerView {
	out := append([]protocol.PlayerView(nil), players...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Score < out[j-1].Score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ---- kicked ----

func (m *Model) renderKicked() string {
	return m.centerBlock(m.kicked + "\n\n" + m.st.faint.Render("press any key to disconnect"))
}

// ---- helpers ----

func windowIndices(n, cursor, maxCells int) (start, end int, left, right bool) {
	if n <= maxCells {
		return 0, n, false, false
	}
	start = cursor - maxCells/2
	if start < 0 {
		start = 0
	}
	if start+maxCells > n {
		start = n - maxCells
	}
	end = start + maxCells
	return start, end, start > 0, end < n
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
