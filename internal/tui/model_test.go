package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Avangelista/deuception/internal/game"
	"github.com/Avangelista/deuception/internal/protocol"
	"github.com/Avangelista/deuception/internal/room"
)

type nopCommander struct{}

func (nopCommander) Submit(room.Command) {}

func parseHand(t *testing.T, s string) []game.Card {
	t.Helper()
	cs, err := game.ParseCards(s)
	if err != nil {
		t.Fatalf("ParseCards(%q): %v", s, err)
	}
	return cs
}

func inGameSnap(rev int, h []game.Card) protocol.StateSnapshot {
	return protocol.StateSnapshot{
		Phase:   protocol.InGame,
		Rev:     rev,
		YouSeat: 0,
		Players: []protocol.PlayerView{
			{Seat: 0, IsYou: true, IsTurn: true, CardCount: len(h)},
		},
		YourHand: h,
		Turn:     0,
		TableBy:  -1,
		Winner:   -1,
	}
}

// TestSelectionResetsOnEqualSizeRedeal: an equal-size redeal must still clear
// stale selection indices.
func TestSelectionResetsOnEqualSizeRedeal(t *testing.T) {
	m := New(nopCommander{}, "id", "hint", lipgloss.DefaultRenderer())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	h1 := parseHand(t, "3D 4H 5C 8D TS JH 2S") // 7 cards
	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(1, h1)})
	m.selected[1] = true
	m.selected[3] = true

	h2 := parseHand(t, "3C 4D 5S 8C TD JD 2C") // 7 different cards
	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(2, h2)})
	if len(m.selected) != 0 {
		t.Fatalf("selection should reset on an equal-size redeal, got %v", m.selected)
	}
}

// TestSelectionPersistsWhenHandUnchanged: an unchanged hand (opponent's move)
// must keep our pending selection.
func TestSelectionPersistsWhenHandUnchanged(t *testing.T) {
	m := New(nopCommander{}, "id", "hint", lipgloss.DefaultRenderer())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	h := parseHand(t, "3D 4H 5C 8D TS JH 2S")
	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(1, h)})
	m.selected[2] = true

	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(2, h)})
	if !m.selected[2] {
		t.Fatalf("selection should persist while the hand is unchanged")
	}
}

// tableSnap is an in-game snapshot with a two-player pile: seat 0 (you) leads, the
// given table combo was just played by tableBy.
func tableSnap(rev int, yourHand, table []game.Card, tableBy int) protocol.StateSnapshot {
	return protocol.StateSnapshot{
		Phase:   protocol.InGame,
		Rev:     rev,
		YouSeat: 0,
		Players: []protocol.PlayerView{
			{Seat: 0, IsYou: true, CardCount: len(yourHand), Connected: true},
			{Seat: 1, CardCount: 5, Connected: true},
		},
		YourHand: yourHand,
		Table:    table,
		TableBy:  tableBy,
		Turn:     0,
		Winner:   -1,
	}
}

// cardCol returns the leftmost column of "|<face>" for face in a rendered frame, or
// -1. Used to track where the pile card sits.
func cardCol(frame, face string) int {
	needle := "|" + face
	best := -1
	for _, line := range splitLines(frame) {
		if i := indexOf(line, needle); i >= 0 {
			if best == -1 || i < best {
				best = i
			}
		}
	}
	return best
}

func splitLines(s string) []string {
	var out, cur = []string{}, []rune{}
	for _, r := range s {
		if r == '\n' {
			out = append(out, string(cur))
			cur = cur[:0]
		} else {
			cur = append(cur, r)
		}
	}
	return append(out, string(cur))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestPileSlidesFromSideToCentre: a play by the top opponent (seat 1, drawn above)
// starts at the top of the mid region and glides to centre; at rest it is centred
// and only the current card shows.
func TestPileSlidesFromSideToCentre(t *testing.T) {
	m := New(nopCommander{}, "id", "hint", lipgloss.DefaultRenderer())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	hand := parseHand(t, "4D 4H 5C 8D TS JH 2S")
	// A 2-player pile: seat 1 is the top opponent, so its play slides down (dy<0
	// origin = top of the block). Lead first so there is a play on the table.
	m.Update(protocol.StateSnapshotMsg{Snap: tableSnap(1, hand, parseHand(t, "3D 3C"), 0)})
	m.SettlePile()
	if m.pileStep != pileSteps {
		t.Fatalf("lead should settle; step=%d want %d", m.pileStep, pileSteps)
	}

	// Now seat 1 (top) beats it. Direction must point up (dy negative).
	m.Update(protocol.StateSnapshotMsg{Snap: tableSnap(2, hand, parseHand(t, "6H 6S"), 1)})
	if m.pileDir != [2]int{0, -1} {
		t.Fatalf("top play direction = %v, want {0,-1}", m.pileDir)
	}
	if m.pileStep != 0 {
		t.Fatalf("new play should start at step 0, got %d", m.pileStep)
	}
	if !sameHand(m.pilePrev, parseHand(t, "3D 3C")) {
		t.Fatalf("previous play not retained for the cover, got %v", m.pilePrev)
	}

	// The card starts fully above the block (off-screen, clipped), so it isn't drawn
	// at step 0. Track the "6H" face row once it appears: it must only move down
	// (a top play slides in from the top) and finish below where it first showed.
	rowOf := func() int {
		for r, line := range splitLines(m.View()) {
			if indexOf(line, "|6H") >= 0 {
				return r
			}
		}
		return -1
	}
	if rowOf() >= 0 {
		t.Fatal("incoming card should start off-screen (clipped), not fully drawn")
	}
	firstRow, lastRow := -1, -1
	for m.pileStep < pileSteps {
		m.Update(pileAnimMsg{gen: m.pileGen})
		if r := rowOf(); r >= 0 {
			if firstRow == -1 {
				firstRow = r
			}
			if lastRow != -1 && r < lastRow {
				t.Fatalf("card moved up (%d -> %d); a top play should slide down", lastRow, r)
			}
			lastRow = r
		}
	}
	if firstRow == -1 {
		t.Fatal("incoming card never slid into view")
	}
	if lastRow <= firstRow {
		t.Fatalf("card did not slide down: first visible row %d, end row %d", firstRow, lastRow)
	}
	if m.pilePrev != nil {
		t.Fatalf("covered play should be dropped once settled, got %v", m.pilePrev)
	}
	// At rest, exactly one pile card (the current 6H/6S) is present, not the old 3s.
	frame := m.View()
	if cardCol(frame, "3D") >= 0 || cardCol(frame, "3C") >= 0 {
		t.Fatal("beaten play still visible at rest; the cover should hide it")
	}
}

// TestStaleSnapshotIgnored: an out-of-order lower-rev snapshot is dropped, not
// applied over current state.
func TestStaleSnapshotIgnored(t *testing.T) {
	m := New(nopCommander{}, "id", "hint", lipgloss.DefaultRenderer())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	newer := parseHand(t, "3D 4H 5C 8D TS JH 2S")
	older := parseHand(t, "3C 4D 5S 8C TD JD 2C")
	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(5, newer)})
	m.Update(protocol.StateSnapshotMsg{Snap: inGameSnap(3, older)}) // arrives late
	if !sameHand(m.snap.YourHand, newer) {
		t.Fatalf("a stale (lower-rev) snapshot must be ignored, got %v", m.snap.YourHand)
	}
}
