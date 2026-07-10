package game

// Event is something that happened as a result of a move; the room layer acts on
// GameWonEvent to score and end the hand.
type Event interface{ isEvent() }

// PlayedEvent: a seat played a combo.
type PlayedEvent struct {
	Seat  Seat
	Combo Combo
}

// PassedEvent: a seat passed (and is now locked out for the trick).
type PassedEvent struct {
	Seat Seat
}

// TrickWonEvent: everyone else passed; Winner takes the trick and leads next.
type TrickWonEvent struct {
	Winner Seat
}

// GameWonEvent: Winner emptied their hand; the hand is over.
type GameWonEvent struct {
	Winner Seat
}

func (PlayedEvent) isEvent()   {}
func (PassedEvent) isEvent()   {}
func (TrickWonEvent) isEvent() {}
func (GameWonEvent) isEvent()  {}
