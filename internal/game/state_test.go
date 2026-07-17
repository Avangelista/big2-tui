package game

import (
	"errors"
	"math/rand"
	"testing"
)

// gameWith builds a started game from explicit space-separated hands, reproducing
// Deal's post-conditions (opening card = lowest dealt, its holder leads).
func gameWith(t *testing.T, hands ...string) *GameState {
	t.Helper()
	g := NewGame(len(hands), DefaultRules())
	for i, h := range hands {
		g.Hands[i] = cards(t, h)
		sortCards(g.Hands[i])
	}
	g.OpenCard = g.lowestDealtCard()
	g.Turn = g.seatWithCard(g.OpenCard)
	g.Leader = g.Turn
	g.Table = nil
	g.Started = true
	g.firstPlay = true
	g.Winner = -1
	return g
}

func mustPlay(t *testing.T, g *GameState, seat Seat, s string) []Event {
	t.Helper()
	ev, err := g.Play(seat, cards(t, s))
	if err != nil {
		t.Fatalf("Play(%d,%q): %v", seat, s, err)
	}
	return ev
}

func mustPass(t *testing.T, g *GameState, seat Seat) []Event {
	t.Helper()
	ev, err := g.Pass(seat)
	if err != nil {
		t.Fatalf("Pass(%d): %v", seat, err)
	}
	return ev
}

func hasEvent[T Event](evs []Event) bool {
	for _, e := range evs {
		if _, ok := e.(T); ok {
			return true
		}
	}
	return false
}

func TestDealFourPlayers(t *testing.T) {
	g := NewGame(4, DefaultRules())
	if err := g.Deal(rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if len(g.Hands[i]) != 13 {
			t.Errorf("seat %d dealt %d cards, want 13", i, len(g.Hands[i]))
		}
	}
	if (g.OpenCard != Card{Rank: Rank3, Suit: Diamond}) {
		t.Errorf("OpenCard = %v, want 3D (always in play with 4 players)", g.OpenCard)
	}
	if !containsCard(g.Hands[g.Turn], g.OpenCard) {
		t.Errorf("leader (seat %d) does not hold the opening card", g.Turn)
	}
	if g.Leader != g.Turn {
		t.Errorf("Leader %d != Turn %d at deal", g.Leader, g.Turn)
	}
}

func TestDealThreePlayers(t *testing.T) {
	// several seeds to exercise the 3D-in-leftover edge case
	for _, seed := range []int64{1, 2, 7, 13, 99, 100, 2024} {
		g := NewGame(3, DefaultRules())
		if err := g.Deal(rand.New(rand.NewSource(seed))); err != nil {
			t.Fatal(err)
		}
		total, eighteens := 0, 0
		for i := 0; i < 3; i++ {
			total += len(g.Hands[i])
			switch len(g.Hands[i]) {
			case 17:
			case 18:
				eighteens++
			default:
				t.Errorf("seed %d: seat %d has %d cards, want 17 or 18", seed, i, len(g.Hands[i]))
			}
		}
		if total != 52 {
			t.Errorf("seed %d: 3-player deal put %d cards in play, want 52 (all dealt)", seed, total)
		}
		if eighteens != 1 {
			t.Errorf("seed %d: %d players hold 18 cards, want exactly 1 (the 3D holder)", seed, eighteens)
		}
		threeD := Card{Rank: Rank3, Suit: Diamond}
		if g.OpenCard != threeD {
			t.Errorf("seed %d: OpenCard = %v, want 3D (all cards dealt)", seed, g.OpenCard)
		}
		if !containsCard(g.Hands[g.Turn], threeD) || len(g.Hands[g.Turn]) != 18 {
			t.Errorf("seed %d: leader should hold 3D and 18 cards", seed)
		}
	}
}

func TestDealTwoPlayers(t *testing.T) {
	g := NewGame(2, DefaultRules())
	if err := g.Deal(rand.New(rand.NewSource(5))); err != nil {
		t.Fatal(err)
	}
	total := 0
	for i := 0; i < 2; i++ {
		if len(g.Hands[i]) != 17 {
			t.Errorf("seat %d dealt %d cards, want 17", i, len(g.Hands[i]))
		}
		total += len(g.Hands[i])
	}
	if total != 34 {
		t.Errorf("2-player deal put %d cards in play, want 34 (18 discarded)", total)
	}
	// Lowest dealt card leads (not necessarily the 3D, which may be discarded).
	if !containsCard(g.Hands[g.Turn], g.OpenCard) {
		t.Errorf("leader does not hold the opening card")
	}
}

func TestMustPlayOpenCard(t *testing.T) {
	g := gameWith(t,
		"3D 4D 5D",
		"3C 4C 5C",
		"3H 4H 5H",
		"3S 4S 5S",
	)
	if (g.OpenCard != Card{Rank: Rank3, Suit: Diamond}) {
		t.Fatalf("OpenCard = %v, want 3D", g.OpenCard)
	}
	if _, err := g.Play(0, cards(t, "4D")); !errors.Is(err, ErrMustPlayOpenCard) {
		t.Errorf("playing without 3D: err = %v, want ErrMustPlayOpenCard", err)
	}
	if _, err := g.Play(0, cards(t, "3D")); err != nil {
		t.Errorf("playing 3D first: unexpected err %v", err)
	}
}

func TestNotYourTurn(t *testing.T) {
	g := gameWith(t, "3D 4D", "3C 4C", "3H 4H", "3S 4S")
	if _, err := g.Play(1, cards(t, "3C")); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("err = %v, want ErrNotYourTurn", err)
	}
}

func TestCannotPassOnLead(t *testing.T) {
	g := gameWith(t, "3D 4D", "3C 4C", "3H 4H", "3S 4S")
	if _, err := g.Pass(0); !errors.Is(err, ErrCannotPassOnLead) {
		t.Errorf("err = %v, want ErrCannotPassOnLead", err)
	}
}

func TestFollowRules(t *testing.T) {
	g := gameWith(t, "3D 4D", "5C 5H 6C", "3H 4H", "3S 4S")
	mustPlay(t, g, 0, "3D") // lead single
	// seat 1 must match size (1); a valid pair is still the wrong size vs a single
	if _, err := g.Play(1, cards(t, "5C 5H")); !errors.Is(err, ErrWrongComboSize) {
		t.Errorf("wrong size: err = %v, want ErrWrongComboSize", err)
	}
	// a single that beats the table
	if _, err := g.Play(1, cards(t, "5C")); err != nil {
		t.Errorf("5C beats 3D: unexpected err %v", err)
	}
}

// TestTrickWithLockedOutPassing walks a full trick: passers stay out, players who
// only played stay active, and the leader takes the trick once all others pass.
func TestTrickWithLockedOutPassing(t *testing.T) {
	g := gameWith(t,
		"3D 4D 5D",
		"3C 4C 5C",
		"3H 4H 5H",
		"3S 4S 5S",
	)
	mustPlay(t, g, 0, "3D") // s0 leads
	if g.Turn != 1 {
		t.Fatalf("after s0 plays, Turn = %d, want 1", g.Turn)
	}
	mustPass(t, g, 1) // s1 out for the trick
	if g.Turn != 2 {
		t.Fatalf("after s1 passes, Turn = %d, want 2", g.Turn)
	}
	mustPlay(t, g, 2, "3H") // beats 3D
	mustPass(t, g, 3)       // s3 out
	if g.Turn != 0 {
		t.Fatalf("Turn = %d, want 0 (s0 played earlier, still active)", g.Turn)
	}
	mustPlay(t, g, 0, "4D") // s0 re-enters and beats 3H
	if g.Turn != 2 {
		t.Fatalf("after s0 plays, Turn = %d, want 2 (s1 locked out)", g.Turn)
	}
	evs := mustPass(t, g, 2) // s2 passes -> only s0 left -> trick over
	if !hasEvent[TrickWonEvent](evs) {
		t.Fatalf("expected TrickWonEvent, got %#v", evs)
	}
	if g.Leader != 0 || g.Turn != 0 {
		t.Fatalf("after trick: Leader=%d Turn=%d, want both 0", g.Leader, g.Turn)
	}
	if g.Table != nil {
		t.Fatalf("Table should be cleared after a trick")
	}
	for i, p := range g.Passed {
		if p {
			t.Fatalf("Passed[%d] should be reset after a trick", i)
		}
	}
}

// TestPassMarkerPersistsUntilTrickEnds verifies a pass stays flagged for the whole
// trick - a later play by someone else does not clear it.
func TestPassMarkerPersistsUntilTrickEnds(t *testing.T) {
	g := gameWith(t,
		"3D 4D 5D",
		"3C 4C 5C",
		"3H 4H 5H",
		"3S 4S 5S",
	)
	mustPlay(t, g, 0, "3D") // s0 leads
	mustPass(t, g, 1)       // s1 passes -> locked out
	mustPlay(t, g, 2, "3H") // a later play must NOT clear s1's pass marker
	if !g.Passed[1] {
		t.Fatalf("s1 stays locked out (Passed[1]) across a later play")
	}
	mustPass(t, g, 3) // s3 passes too
	if !g.Passed[1] || !g.Passed[3] {
		t.Fatalf("both passers stay marked until the trick ends: Passed=%v", g.Passed)
	}
}

func TestGameWonAndScores(t *testing.T) {
	g := gameWith(t,
		"3D",       // s0 will empty and win
		"4C 5C",    // 2 left
		"6H 7H 8H", // 3 left
		"9S TS",    // 2 left
	)
	evs := mustPlay(t, g, 0, "3D")
	if !hasEvent[GameWonEvent](evs) {
		t.Fatalf("expected GameWonEvent, got %#v", evs)
	}
	if !g.Finished || g.Winner != 0 {
		t.Fatalf("Finished=%v Winner=%d, want true/0", g.Finished, g.Winner)
	}
	got := g.HandScores()
	want := []int{0, 2, 3, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("HandScores[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestPenalty(t *testing.T) {
	tests := map[int]int{0: 0, 1: 1, 7: 7, 8: 16, 9: 18, 10: 30, 12: 36, 13: 52}
	for in, want := range tests {
		if got := Penalty(in); got != want {
			t.Errorf("Penalty(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPlayAfterFinishedRejected(t *testing.T) {
	g := gameWith(t, "3D", "4C 5C", "6H 7H", "8S 9S")
	mustPlay(t, g, 0, "3D") // game over
	if _, err := g.Play(1, cards(t, "4C")); !errors.Is(err, ErrGameNotActive) {
		t.Errorf("err = %v, want ErrGameNotActive", err)
	}
}
