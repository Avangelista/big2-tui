package game

import "math/rand"

// Seat identifies a player position, 0..NumSeats-1.
type Seat int

// GameState is the authoritative state of a single hand. Not safe for concurrent
// use: the room layer owns it from one goroutine.
type GameState struct {
	NumSeats int
	Hands    [][]Card // per seat, sorted ascending by Order
	Turn     Seat
	Table    *Combo // current winning play; nil starts a new trick (Turn must lead)
	Leader   Seat   // seat that owns Table (last to play)
	Passed   []bool // a pass locks the seat out until the trick resets
	Started  bool
	Finished bool
	Winner   Seat // -1 until someone empties their hand
	OpenCard Card // mandatory card for the hand's first play

	firstPlay bool
	rules     Rules
}

// NewGame creates a game for numSeats players under r (the zero value is classic rules).
func NewGame(numSeats int, r Rules) *GameState {
	return &GameState{
		NumSeats: numSeats,
		Hands:    make([][]Card, numSeats),
		Passed:   make([]bool, numSeats),
		Winner:   -1,
		rules:    r,
	}
}

// Rules returns the game's ruleset.
func (g *GameState) Rules() Rules { return g.rules }

// Deal shuffles with rng and deals a fresh hand: 13 each (4p), 17 each plus the
// leftover to the 3D holder (3p), or 17 each with 18 discarded (2p). The first
// play must include the opening card - the lowest card dealt, which is the 3D
// unless it landed in the 2-player discard; its holder leads.
func (g *GameState) Deal(rng *rand.Rand) error {
	if g.NumSeats < 2 || g.NumSeats > 4 {
		return ErrBadPlayerCount
	}
	threeD := Card{Rank: Rank3, Suit: Diamond}
	deck := Deck()
	rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	switch g.NumSeats {
	case 4:
		for i := 0; i < 4; i++ {
			g.Hands[i] = append([]Card(nil), deck[i*13:(i+1)*13]...)
		}
	case 3:
		// ensure the 3D is dealt, not left over, so it has a definite holder
		if deck[51] == threeD {
			deck[51], deck[0] = deck[0], deck[51]
		}
		for i := 0; i < 3; i++ {
			g.Hands[i] = append([]Card(nil), deck[i*17:(i+1)*17]...)
		}
		h := g.seatWithCard(threeD)
		g.Hands[h] = append(g.Hands[h], deck[51])
	case 2:
		for i := 0; i < 2; i++ {
			g.Hands[i] = append([]Card(nil), deck[i*17:(i+1)*17]...)
		}
	}
	for i := 0; i < g.NumSeats; i++ {
		sortCards(g.Hands[i])
	}
	g.OpenCard = g.lowestDealtCard()
	g.Turn = g.seatWithCard(g.OpenCard)
	g.Leader = g.Turn
	g.Table = nil
	for i := range g.Passed {
		g.Passed[i] = false
	}
	g.Started = true
	g.Finished = false
	g.Winner = -1
	g.firstPlay = true
	return nil
}

// FirstPlay reports whether the next play is the hand's opening play, which must
// include OpenCard.
func (g *GameState) FirstPlay() bool { return g.firstPlay }

// LeadFrom makes seat open the current hand with a free play (no open-card rule), used
// for the winner-leads rule. Call after Deal.
func (g *GameState) LeadFrom(seat Seat) {
	g.Turn = seat
	g.Leader = seat
	g.firstPlay = false
}

// Play validates and applies seat's play, returning events or an error (state
// unchanged on error).
func (g *GameState) Play(seat Seat, cards []Card) ([]Event, error) {
	if !g.Started || g.Finished {
		return nil, ErrGameNotActive
	}
	if seat != g.Turn {
		return nil, ErrNotYourTurn
	}
	combo, err := Classify(cards, g.rules)
	if err != nil {
		return nil, err
	}
	if !g.ownsAll(seat, combo.Cards) {
		return nil, ErrDontOwnCards
	}
	if g.firstPlay && !containsCard(combo.Cards, g.OpenCard) {
		return nil, ErrMustPlayOpenCard
	}
	if g.Table != nil {
		if len(combo.Cards) != len(g.Table.Cards) {
			return nil, ErrWrongComboSize
		}
		if !combo.Beats(*g.Table, g.rules) {
			return nil, ErrDoesNotBeat
		}
	}

	g.removeCards(seat, combo.Cards)
	played := combo
	g.Table = &played
	g.Leader = seat
	g.firstPlay = false
	if g.rules.Pass == PassReenter {
		// A fresh play reopens the round: anyone who skipped can respond again.
		for i := range g.Passed {
			g.Passed[i] = false
		}
	}

	events := []Event{PlayedEvent{Seat: seat, Combo: played}}
	if len(g.Hands[seat]) == 0 {
		g.Finished = true
		g.Winner = seat
		events = append(events, GameWonEvent{Winner: seat})
		return events, nil
	}
	events = append(events, g.advance()...)
	return events, nil
}

// Pass applies seat passing; the trick leader may not pass.
func (g *GameState) Pass(seat Seat) ([]Event, error) {
	if !g.Started || g.Finished {
		return nil, ErrGameNotActive
	}
	if seat != g.Turn {
		return nil, ErrNotYourTurn
	}
	if g.Table == nil {
		return nil, ErrCannotPassOnLead
	}
	g.Passed[seat] = true
	events := []Event{PassedEvent{Seat: seat}}
	events = append(events, g.advance()...)
	return events, nil
}

// advance moves the turn to the next active seat. If it wraps back to the leader
// (everyone else passed), the leader takes the trick and leads a fresh one.
func (g *GameState) advance() []Event {
	s := g.Turn
	for i := 0; i < g.NumSeats; i++ {
		s = Seat((int(s) + 1) % g.NumSeats)
		if s == g.Leader {
			break
		}
		if !g.Passed[s] && len(g.Hands[s]) > 0 {
			g.Turn = s
			return nil
		}
	}
	g.Table = nil
	for i := range g.Passed {
		g.Passed[i] = false
	}
	g.Turn = g.Leader
	return []Event{TrickWonEvent{Winner: g.Leader}}
}

func (g *GameState) lowestDealtCard() Card {
	best := Card{Rank: Rank2, Suit: Spade}
	for i := 0; i < g.NumSeats; i++ {
		for _, c := range g.Hands[i] {
			if c.Order() < best.Order() {
				best = c
			}
		}
	}
	return best
}

func (g *GameState) seatWithCard(c Card) Seat {
	for i := 0; i < g.NumSeats; i++ {
		if containsCard(g.Hands[i], c) {
			return Seat(i)
		}
	}
	return 0
}

func (g *GameState) ownsAll(seat Seat, cards []Card) bool {
	for _, c := range cards {
		if !containsCard(g.Hands[seat], c) {
			return false
		}
	}
	return true
}

func (g *GameState) removeCards(seat Seat, cards []Card) {
	remove := make(map[Card]bool, len(cards))
	for _, c := range cards {
		remove[c] = true
	}
	filtered := g.Hands[seat][:0]
	for _, c := range g.Hands[seat] {
		if !remove[c] {
			filtered = append(filtered, c)
		}
	}
	g.Hands[seat] = filtered
}
