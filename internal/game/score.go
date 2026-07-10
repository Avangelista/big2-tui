package game

// Penalty scores a hand still holding cardsLeft cards; the multiplier climbs with
// hand size, so lower cumulative totals are better.
func Penalty(cardsLeft int) int {
	switch {
	case cardsLeft <= 0:
		return 0
	case cardsLeft <= 7:
		return cardsLeft
	case cardsLeft <= 9:
		return cardsLeft * 2
	case cardsLeft <= 12:
		return cardsLeft * 3
	default:
		return cardsLeft * 4
	}
}

// HandScores returns each seat's penalty for the finished hand (winner scores 0).
func (g *GameState) HandScores() []int {
	out := make([]int, g.NumSeats)
	for i := 0; i < g.NumSeats; i++ {
		out[i] = Penalty(len(g.Hands[i]))
	}
	return out
}
