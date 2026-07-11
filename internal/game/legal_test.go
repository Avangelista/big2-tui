package game

import "testing"

func validSize(n int) bool { return n == 1 || n == 2 || n == 3 || n == 5 }

// TestLegalPlaysLeadOpener checks that on the very first play every enumerated
// combo contains the opening card and is one Play would accept.
func TestLegalPlaysLeadOpener(t *testing.T) {
	build := func() *GameState { return gameWith(t, "3D 4D 5D 6D 7D", "8C 9C TC JC QC") }
	g := build()
	plays := g.LegalPlays(0)
	if len(plays) == 0 {
		t.Fatal("no legal plays on lead")
	}
	for _, p := range plays {
		if !validSize(len(p.Cards)) {
			t.Errorf("combo has illegal size %d: %v", len(p.Cards), p.Cards)
		}
		if !containsCard(p.Cards, g.OpenCard) {
			t.Errorf("first-play combo %v omits the open card %v", p.Cards, g.OpenCard)
		}
		// Everything LegalPlays returns must survive Play's own validation.
		fresh := build()
		if _, err := fresh.Play(0, p.Cards); err != nil {
			t.Errorf("Play rejected an enumerated legal move %v: %v", p.Cards, err)
		}
	}
}

// followPos builds a mid-trick position with a pair of 4s on the table and seat 0
// to move, holding a losing pair of 3s and a winning pair of 5s.
func followPos(t *testing.T) *GameState {
	t.Helper()
	g := gameWith(t, "3C 3S 5D 5H", "4H 4S 6C 7C")
	g.firstPlay = false
	tbl, err := Classify(cards(t, "4H 4S"), SimpleStraight)
	if err != nil {
		t.Fatal(err)
	}
	g.Table = &tbl
	g.Turn, g.Leader = 0, 1
	return g
}

// TestLegalPlaysFollow: following, only same-size combos that beat the table are
// returned - here just the pair of 5s.
func TestLegalPlaysFollow(t *testing.T) {
	g := followPos(t)
	plays := g.LegalPlays(0)
	if len(plays) != 1 {
		t.Fatalf("got %d legal plays, want 1 (the pair of 5s)", len(plays))
	}
	for _, p := range plays {
		if len(p.Cards) != len(g.Table.Cards) {
			t.Errorf("combo %v does not match the table size %d", p.Cards, len(g.Table.Cards))
		}
		if !p.Beats(*g.Table) {
			t.Errorf("combo %v does not beat the table", p.Cards)
		}
	}
	if got := CardsString(plays[0].Cards); got != "5D,5H" {
		t.Errorf("legal play = %s, want 5D,5H", got)
	}
}

// TestLegalPlaysPassOnly: when nothing beats the table, LegalPlays is empty, which
// the bot reads as "must pass".
func TestLegalPlaysPassOnly(t *testing.T) {
	g := gameWith(t, "3C 3S 8D", "4H 4S 6C")
	g.firstPlay = false
	tbl, err := Classify(cards(t, "4H 4S"), SimpleStraight)
	if err != nil {
		t.Fatal(err)
	}
	g.Table = &tbl
	g.Turn, g.Leader = 0, 1
	if plays := g.LegalPlays(0); len(plays) != 0 {
		t.Fatalf("got %d legal plays, want 0 (only a losing pair of 3s): %v", len(plays), plays)
	}
}

// TestLegalPlaysNoBareQuad: a four-of-a-kind hand yields the 5-card FourKind combo
// but never a bare 4-card play.
func TestLegalPlaysNoBareQuad(t *testing.T) {
	g := gameWith(t, "4D 4C 4H 4S 7D 8D", "9C TC JC QC KC AC 2C")
	g.firstPlay = false // lead, no open-card constraint
	plays := g.LegalPlays(0)
	quads := 0
	for _, p := range plays {
		if !validSize(len(p.Cards)) {
			t.Errorf("combo has illegal size %d: %v", len(p.Cards), p.Cards)
		}
		if p.Type == FourKind {
			quads++
		}
	}
	if quads == 0 {
		t.Error("expected at least one FourKind combo from four 4s")
	}
}
