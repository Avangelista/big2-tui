// Package game is the pure Big 2 rules engine: cards, combinations, and the
// per-hand state machine. No I/O; standard library only.
package game

import (
	"fmt"
	"sort"
	"strings"
)

// Rank is a card rank. Enum order is the Big 2 comparison order: 3 lowest, 2 highest.
type Rank uint8

const (
	Rank3 Rank = iota
	Rank4
	Rank5
	Rank6
	Rank7
	Rank8
	Rank9
	Rank10
	RankJ
	RankQ
	RankK
	RankA
	Rank2
)

// Suit is a card suit. Enum order is the comparison order: Diamond < Club < Heart < Spade.
type Suit uint8

const (
	Diamond Suit = iota
	Club
	Heart
	Spade
)

// Card is a single playing card. Cards are unique within a deck.
type Card struct {
	Rank Rank
	Suit Suit
}

// Order is a total order over cards, rank-major then suit-minor: 3D is 0 (lowest), 2S highest.
func (c Card) Order() int { return int(c.Rank)*4 + int(c.Suit) }

var rankLabels = [...]string{"3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A", "2"}
var suitLabels = [...]string{"D", "C", "H", "S"}

// String renders a rank; ten is "T".
func (r Rank) String() string { return rankLabels[r] }

func (s Suit) String() string { return suitLabels[s] }

// String renders a card as a 2-char token, e.g. "TS".
func (c Card) String() string { return rankLabels[c.Rank] + suitLabels[c.Suit] }

// Deck returns a fresh, ordered 52-card deck.
func Deck() []Card {
	d := make([]Card, 0, 52)
	for r := Rank3; r <= Rank2; r++ {
		for s := Diamond; s <= Spade; s++ {
			d = append(d, Card{Rank: r, Suit: s})
		}
	}
	return d
}

func sortCards(cs []Card) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Order() < cs[j].Order() })
}

// SortCards sorts cards ascending by Order, in place.
func SortCards(cs []Card) { sortCards(cs) }

func containsCard(cs []Card, c Card) bool {
	for _, x := range cs {
		if x == c {
			return true
		}
	}
	return false
}

// CardsString renders cards as a comma-separated list, e.g. "6D,6H,6S".
func CardsString(cs []Card) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.String()
	}
	return strings.Join(parts, ",")
}

func rankFromByte(b byte) (Rank, bool) {
	for i, lbl := range rankLabels {
		if lbl[0] == b {
			return Rank(i), true
		}
	}
	return 0, false
}

func suitFromByte(b byte) (Suit, bool) {
	for i, lbl := range suitLabels {
		if lbl[0] == b {
			return Suit(i), true
		}
	}
	return 0, false
}

// ParseCard parses a token like "TD" or "2S" (case-insensitive) into a Card.
func ParseCard(tok string) (Card, error) {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	if len(tok) != 2 {
		return Card{}, fmt.Errorf("bad card token %q", tok)
	}
	r, ok := rankFromByte(tok[0])
	if !ok {
		return Card{}, fmt.Errorf("bad rank in %q", tok)
	}
	s, ok := suitFromByte(tok[1])
	if !ok {
		return Card{}, fmt.Errorf("bad suit in %q", tok)
	}
	return Card{Rank: r, Suit: s}, nil
}

// ParseCards parses a whitespace- or comma-separated list of card tokens.
func ParseCards(s string) ([]Card, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	out := make([]Card, 0, len(fields))
	for _, f := range fields {
		c, err := ParseCard(f)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
