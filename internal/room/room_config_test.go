package room

import (
	mrand "math/rand"
	"testing"

	"github.com/Avangelista/big2-tui/internal/game"
	"github.com/Avangelista/big2-tui/internal/protocol"
)

func TestSetRulesHostOnlyAndLocked(t *testing.T) {
	r := New(4, 3, mrand.New(mrand.NewSource(1)))
	ids := joinN(r, 3)

	want := game.Rules{Straights: game.StraightsPoker, Flush: game.FlushBySuit, Pass: game.PassReenter, Lead: game.LeadWinner}

	// A non-host cannot change the rules.
	r.Submit(SetRulesCmd{ID: ids[1], Rules: want})
	if got := r.Query(ids[0]).Rules; got != (game.Rules{}) {
		t.Errorf("non-host set rules to %+v, want them unchanged (zero)", got)
	}

	// The host can, and every viewer sees it.
	r.Submit(SetRulesCmd{ID: ids[0], Rules: want})
	for _, id := range ids {
		if got := r.Query(id).Rules; got != want {
			t.Errorf("after host set: Rules = %+v, want %+v", got, want)
		}
	}

	// Once the game starts the rules are locked.
	r.Submit(StartCmd{ID: ids[0]})
	r.Submit(SetRulesCmd{ID: ids[0], Rules: game.Rules{}})
	if got := r.Query(ids[0]).Rules; got != want {
		t.Errorf("rules changed mid-game to %+v, want them locked at %+v", got, want)
	}
}

func TestSetReaction(t *testing.T) {
	r := New(4, 3, mrand.New(mrand.NewSource(2)))
	ids := joinN(r, 3)

	// Defaults match protocol.Emotes.
	snap := r.Query(ids[0])
	if len(snap.Reactions) != len(protocol.Emotes) {
		t.Fatalf("Reactions len = %d, want %d", len(snap.Reactions), len(protocol.Emotes))
	}
	for i, want := range protocol.Emotes {
		if snap.Reactions[i] != want {
			t.Errorf("default Reactions[%d] = %q, want %q", i, snap.Reactions[i], want)
		}
	}

	// Host sets a label; it is visible room-wide.
	r.Submit(SetReactionCmd{ID: ids[0], Index: 2, Text: "yo"})
	if got := r.Query(ids[1]).Reactions[2]; got != "yo" {
		t.Errorf("Reactions[2] = %q, want %q", got, "yo")
	}

	// Too long (>5 runes) is rejected, leaving the label unchanged.
	r.Submit(SetReactionCmd{ID: ids[0], Index: 2, Text: "toolong"})
	if got := r.Query(ids[0]).Reactions[2]; got != "yo" {
		t.Errorf("over-long label took effect: Reactions[2] = %q, want %q", got, "yo")
	}

	// A non-host cannot change labels.
	r.Submit(SetReactionCmd{ID: ids[1], Index: 0, Text: "no"})
	if got := r.Query(ids[0]).Reactions[0]; got != protocol.Emotes[0] {
		t.Errorf("non-host changed a label: Reactions[0] = %q, want %q", got, protocol.Emotes[0])
	}

	// Out-of-range index is a no-op (no panic).
	r.Submit(SetReactionCmd{ID: ids[0], Index: 99, Text: "x"})
	if got := len(r.Query(ids[0]).Reactions); got != len(protocol.Emotes) {
		t.Errorf("Reactions len changed to %d after bad index", got)
	}

	// Locked once the game starts.
	r.Submit(StartCmd{ID: ids[0]})
	r.Submit(SetReactionCmd{ID: ids[0], Index: 2, Text: "hi"})
	if got := r.Query(ids[0]).Reactions[2]; got != "yo" {
		t.Errorf("label changed mid-game to %q, want locked at %q", got, "yo")
	}
}

// TestWinnerLeadsNextHand checks the LeadWinner rule: the previous hand's winner opens
// the next hand freely (no forced open card, and they hold the turn).
func TestWinnerLeadsNextHand(t *testing.T) {
	r := New(4, 3, mrand.New(mrand.NewSource(7)))
	ids := joinN(r, 4)
	r.Submit(SetRulesCmd{ID: ids[0], Rules: game.Rules{Lead: game.LeadWinner}})
	r.Submit(StartCmd{ID: ids[0]})

	playOutHand(t, r, ids)
	final := r.Query(ids[0])
	if final.Phase != protocol.Finished {
		t.Fatalf("hand did not finish; phase = %v", final.Phase)
	}
	winner := final.Winner
	if winner < 0 {
		t.Fatalf("no winner recorded: %d", winner)
	}

	r.Submit(NextHandCmd{ID: ids[0]})
	next := r.Query(ids[0])
	if next.Phase != protocol.InGame {
		t.Fatalf("next hand did not start; phase = %v", next.Phase)
	}
	if next.Turn != winner {
		t.Errorf("next hand opens on seat %d, want the previous winner (seat %d)", next.Turn, winner)
	}
	// The winner opens freely: no mandatory open card.
	if w := r.Query(ids[winner]); w.Opening {
		t.Errorf("winner's opening play is still forced to a card; want a free lead")
	}
}
