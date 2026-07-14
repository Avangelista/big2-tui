package game

import (
	"sort"
	"strings"
	"testing"
)

func mustCards(t *testing.T, s string) []Card {
	t.Helper()
	cs, err := ParseCards(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	sortCards(cs)
	return cs
}

// playableStr returns the space-joined, sorted set of hand cards reported playable.
func playableStr(hand, selected, table []Card, opening bool, openCard Card) string {
	ps := PlayableSet(hand, selected, table, opening, openCard)
	var out []string
	for i, c := range hand {
		if ps[i] {
			out = append(out, c.String())
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func TestPlayableSet(t *testing.T) {
	c := func(s string) []Card { return mustCards(t, s) }
	card := func(s string) Card { return c(s)[0] }

	cases := []struct {
		name     string
		hand     string
		selected string
		table    string
		opening  bool
		open     string
		want     string // sorted playable cards
	}{
		{
			name: "single ten: only higher singles",
			hand: "3D 5C 9H TS JD KD 2S", table: "TC",
			// TS (ten of spades) beats TC by suit; J/K/2 beat by rank.
			want: "2S JD KD TS",
		},
		{
			name: "select a jack over a ten: only that single",
			hand: "3D 5C 9H TS JD KD 2S", selected: "JD", table: "TC",
			want: "JD",
		},
		{
			name: "lead, select 3S: 3s (pair/triple), spades (flush), 4-7 (straight); not 9C",
			hand: "3S 3D 3C 4S 5S 6S 7S 9C", selected: "3S",
			want: "3C 3D 3S 4S 5S 6S 7S",
		},
		{
			name: "lead, select two 3s: flush gone, only the third 3 (triple) stays",
			hand: "3S 3D 3C 4S 5S 6S 7S 9C", selected: "3S 3D",
			want: "3C 3D 3S",
		},
		{
			name: "opening play: only cards that combine with 3D",
			hand: "3D 4D 5D 6D 7D KS 2S", opening: true, open: "3D",
			// single 3D, or a diamond flush / 3-4-5-6-7 straight through 3D; K/2 can't join.
			want: "3D 4D 5D 6D 7D",
		},
		{
			name: "must pass: nothing beats a pair of 2s",
			hand: "3D 4C 5H", table: "2S 2D",
			want: "",
		},
		{
			name: "leading with nothing selected: every card is a legal single",
			hand: "3D 5C 9H KS", want: "3D 5C 9H KS",
		},
		{
			name: "beat a pair: only cards that form a higher pair",
			hand: "5D 5C 7H 7S 9D", table: "6H 6S",
			// 7H7S makes a higher pair; 5s make a lower pair (can't beat); 9D has no partner.
			want: "7H 7S",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var open Card
			if tc.open != "" {
				open = card(tc.open)
			}
			var sel, tbl []Card
			if tc.selected != "" {
				sel = c(tc.selected)
			}
			if tc.table != "" {
				tbl = c(tc.table)
			}
			got := playableStr(c(tc.hand), sel, tbl, tc.opening, open)
			if got != tc.want {
				t.Errorf("playable = %q, want %q", got, tc.want)
			}
		})
	}
}
