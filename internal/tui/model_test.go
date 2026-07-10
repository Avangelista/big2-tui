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
