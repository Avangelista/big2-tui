package game

// PlayableSet reports, for each card in hand, whether adding it to the current
// selection can still complete a legal play: one that beats the table combo (table
// empty means leading), and that includes openCard when opening is set. hand must be
// the player's hand; selected must be a subset of it. Selected cards always report
// true (they can be deselected). A card that reports false is unplayable (greyed) and
// must not be selectable. r must be the game's ruleset so the enumeration matches what
// the room will accept.
//
// It enumerates every legal completion of the selection - size fixed to the table's
// size when beating, any of {1,2,3,5} when leading - and marks the cards of each. The
// hand is at most 13 cards, so the enumeration (bounded by C(13,5)) is cheap enough
// to run on every selection change.
func PlayableSet(hand, selected, table []Card, opening bool, openCard Card, r Rules) []bool {
	playable := make([]bool, len(hand))
	idxOf := make(map[Card]int, len(hand))
	for i, c := range hand {
		idxOf[c] = i
	}
	selSet := make(map[Card]bool, len(selected))
	for _, c := range selected {
		selSet[c] = true
	}
	// Cards still available to add.
	var pool []Card
	for _, c := range hand {
		if !selSet[c] {
			pool = append(pool, c)
		}
	}

	// Candidate play sizes: the table's size when beating, else every legal size.
	var tbl *Combo
	sizes := []int{1, 2, 3, 5}
	if len(table) > 0 {
		if c, err := Classify(table, r); err == nil {
			tbl = &c
			sizes = []int{len(table)}
		}
	}

	mark := func(cards []Card) {
		for _, c := range cards {
			if i, ok := idxOf[c]; ok {
				playable[i] = true
			}
		}
	}
	for _, s := range sizes {
		need := s - len(selected)
		if need < 0 {
			continue // selection already exceeds this size
		}
		combinations(pool, need, func(extra []Card) {
			play := make([]Card, 0, s)
			play = append(play, selected...)
			play = append(play, extra...)
			combo, err := Classify(play, r)
			if err != nil {
				return
			}
			if opening && !containsCard(play, openCard) {
				return
			}
			if tbl != nil && !combo.Beats(*tbl, r) {
				return
			}
			mark(play)
		})
	}
	// Selected cards are always deselectable.
	mark(selected)
	return playable
}

// combinations calls fn with every k-card combination of cards (fn's slice is only
// valid for the call). k==0 yields one empty combination.
func combinations(cards []Card, k int, fn func([]Card)) {
	if k == 0 {
		fn(nil)
		return
	}
	n := len(cards)
	if k > n {
		return
	}
	idx := make([]int, k)
	for i := range idx {
		idx[i] = i
	}
	pick := make([]Card, k)
	for {
		for i, x := range idx {
			pick[i] = cards[x]
		}
		fn(pick)
		i := k - 1
		for i >= 0 && idx[i] == n-k+i {
			i--
		}
		if i < 0 {
			return
		}
		idx[i]++
		for j := i + 1; j < k; j++ {
			idx[j] = idx[j-1] + 1
		}
	}
}
