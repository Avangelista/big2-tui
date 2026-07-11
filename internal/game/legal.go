package game

// LegalPlays enumerates every legal move for seat under the current table. On
// lead (Table==nil) it always returns at least one combo; when following, an
// empty result means the only legal move is to pass. Anything returned here is
// guaranteed to pass Play's validation.
func (g *GameState) LegalPlays(seat Seat) []Combo {
	hand := g.Hands[seat]
	sizes := []int{1, 2, 3, 5}
	if g.Table != nil {
		sizes = []int{len(g.Table.Cards)} // must match the table count
	}
	var out []Combo
	for _, n := range sizes {
		if n > len(hand) {
			continue
		}
		forEachCombination(len(hand), n, func(idx []int) {
			pick := make([]Card, n)
			for i, j := range idx {
				pick[i] = hand[j]
			}
			combo, err := Classify(pick, g.sc)
			if err != nil {
				return
			}
			if g.firstPlay && !containsCard(combo.Cards, g.OpenCard) {
				return
			}
			if g.Table != nil && !combo.Beats(*g.Table) {
				return
			}
			out = append(out, combo)
		})
	}
	return out
}

// forEachCombination calls fn with each n-index combination of [0, total),
// in lexicographic order.
func forEachCombination(total, n int, fn func(idx []int)) {
	if n <= 0 || n > total {
		return
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	for {
		fn(idx)
		i := n - 1
		for i >= 0 && idx[i] == total-n+i {
			i--
		}
		if i < 0 {
			return
		}
		idx[i]++
		for j := i + 1; j < n; j++ {
			idx[j] = idx[j-1] + 1
		}
	}
}
