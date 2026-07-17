package game

import (
	"errors"
	"testing"
)

func cards(t *testing.T, s string) []Card {
	t.Helper()
	cs, err := ParseCards(s)
	if err != nil {
		t.Fatalf("ParseCards(%q): %v", s, err)
	}
	return cs
}

func mustClassify(t *testing.T, s string) Combo {
	t.Helper()
	c, err := Classify(cards(t, s), DefaultRules())
	if err != nil {
		t.Fatalf("Classify(%q): %v", s, err)
	}
	return c
}

func TestClassifyTypes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want ComboType
	}{
		{"single", "4D", Single},
		{"pair", "4D 4H", Pair},
		{"triple", "6D 6H 6S", Triple},
		{"straight low", "3D 4C 5H 6S 7D", Straight},
		{"straight to ace", "TD JC QH KS AD", Straight},
		{"straight to two", "JD QC KH AS 2D", Straight},
		{"flush", "3D 5D 7D 9D JD", Flush},
		{"full house", "6D 6H 6S 4D 4H", FullHouse},
		{"four of a kind", "9D 9C 9H 9S 3D", FourKind},
		{"straight flush", "3D 4D 5D 6D 7D", StraightFlush},
		{"straight flush to two", "JS QS KS AS 2S", StraightFlush},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustClassify(t, tc.in)
			if got.Type != tc.want {
				t.Errorf("Classify(%q).Type = %v, want %v", tc.in, got.Type, tc.want)
			}
		})
	}
}

func TestClassifyInvalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"two different ranks", "4D 5D", ErrNotAPair},
		{"three not equal", "4D 4H 5S", ErrNotATriple},
		{"four cards", "3D 4D 5D 6D", ErrBadSize},
		{"junk five", "3D 4H 5S 7C 9D", ErrNoFiveCombo},
		{"ace-low not a straight", "AD 2C 3H 4S 5D", ErrNoFiveCombo},
		{"two-low not a straight", "2D 3C 4H 5S 6D", ErrNoFiveCombo},
		{"duplicate card", "4D 4D", ErrDuplicateCard},
		{"empty", "", ErrEmptyPlay},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Classify(cards(t, tc.in), DefaultRules())
			if !errors.Is(err, tc.want) {
				t.Errorf("Classify(%q) err = %v, want %v", tc.in, err, tc.want)
			}
		})
	}
}

func TestBeats(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		// singles: rank then suit
		{"higher rank single", "5D", "4S", true},
		{"same rank higher suit", "2S", "2H", true},
		{"same rank lower suit", "2H", "2S", false},
		// pairs / triples by rank
		{"higher pair", "5D 5C", "4D 4H", true},
		{"lower pair", "4D 4H", "5D 5C", false},
		{"higher triple", "7D 7C 7H", "6D 6C 6H", true},
		// five-card category hierarchy
		{"flush beats straight", "3D 5D 7D 9D JD", "3C 4D 5H 6S 7D", true},
		{"full house beats flush", "6D 6H 6S 4D 4H", "3D 5D 7D 9D JD", true},
		{"quad beats full house", "9D 9C 9H 9S 3D", "6D 6H 6S 4D 4H", true},
		{"straight flush beats quad", "3D 4D 5D 6D 7D", "9D 9C 9H 9S 3H", true},
		// straights compared by top card
		{"straight to ace beats to king", "TD JC QH KS AD", "9D TC JH QS KD", true},
		{"straight to two beats to ace", "JD QC KH AS 2D", "TD JC QH KS AD", true},
		// straights with the same top rank -> the top card's suit breaks the tie
		{"straight same top, higher suit wins", "3D 4C 5H 6S 7C", "3S 4H 5D 6C 7D", true},
		{"straight same top, lower suit loses", "3S 4H 5D 6C 7D", "3D 4C 5H 6S 7C", false},
		// straight flushes with the same top rank -> top card's suit breaks the tie
		{"straight flush same top, higher suit wins", "3S 4S 5S 6S 7S", "3D 4D 5D 6D 7D", true},
		{"straight flush same top, lower suit loses", "3D 4D 5D 6D 7D", "3S 4S 5S 6S 7S", false},
		// flush rank-first: higher top card wins regardless of suit
		{"flush higher top rank wins", "3D 5D 7D 9D KD", "4H 6H 8H TH QH", true},
		// flush same ranks -> suit of top card breaks tie
		{"flush suit tiebreak", "3H 5H 7H 9H KH", "3D 5D 7D 9D KD", true},
		// full house by triple rank; quad by quad rank
		{"full house triple rank", "9D 9C 9H 4D 4H", "8D 8C 8H KD KH", true},
		{"quad by quad rank", "9D 9C 9H 9S 3D", "8D 8C 8H 8S KD", true},
		// size mismatch is never comparable
		{"size mismatch", "4D", "4D 4H", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := mustClassify(t, tc.a)
			b := mustClassify(t, tc.b)
			if got := a.Beats(b, DefaultRules()); got != tc.want {
				t.Errorf("(%q).Beats(%q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestBeatsStrictlyIncreasing checks that in each ascending chain every combo
// beats all earlier ones and loses to all later ones - catching a reversed Beats
// that a direction-blind "exactly one wins" check would miss.
func TestBeatsStrictlyIncreasing(t *testing.T) {
	groups := [][]string{
		// singles: rank then suit
		{"3D", "3S", "4D", "TD", "AD", "2D", "2S"},
		// pairs: by rank
		{"3D 3C", "4D 4C", "TD TC", "AD AC", "2D 2C"},
		// triples: by rank
		{"3D 3C 3H", "7D 7C 7H", "AD AC AH", "2D 2C 2H"},
		// five-card: straight < flush < full house < four-of-a-kind < straight
		// flush; and within a category by the relevant key.
		{
			"3D 4C 5H 6S 7D", // straight to 7
			"TD JC QH KS AD", // straight to A
			"3D 5D 7D 9D JD", // flush (top J)
			"3H 5H 7H 9H KH", // flush (top K) beats flush top J, rank-first
			"6D 6H 6S 4D 4H", // full house 6s
			"9D 9C 9H 4D 4H", // full house 9s
			"8D 8C 8H 8S 3D", // quad 8s
			"3D 4D 5D 6D 7D", // straight flush to 7
			"JS QS KS AS 2S", // straight flush to 2 (highest)
		},
	}
	for _, g := range groups {
		for i := 0; i < len(g); i++ {
			for j := i + 1; j < len(g); j++ {
				lo := mustClassify(t, g[i])
				hi := mustClassify(t, g[j])
				if !hi.Beats(lo, DefaultRules()) {
					t.Errorf("%q should beat %q", g[j], g[i])
				}
				if lo.Beats(hi, DefaultRules()) {
					t.Errorf("%q should NOT beat %q", g[i], g[j])
				}
			}
		}
	}
}
