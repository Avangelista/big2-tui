package game

// ComboType classifies a legal play. For five-card hands the enum order is the
// category-strength order (Straight < Flush < FullHouse < FourKind < StraightFlush).
type ComboType uint8

const (
	Invalid ComboType = iota
	Single
	Pair
	Triple
	Straight
	Flush
	FullHouse
	FourKind
	StraightFlush
)

func (t ComboType) String() string {
	switch t {
	case Single:
		return "single"
	case Pair:
		return "pair"
	case Triple:
		return "triple"
	case Straight:
		return "straight"
	case Flush:
		return "flush"
	case FullHouse:
		return "full house"
	case FourKind:
		return "four of a kind"
	case StraightFlush:
		return "straight flush"
	default:
		return "invalid"
	}
}

// Combo is a classified, legal play. Cards is sorted ascending by Order. Key is
// the comparison card, whose meaning varies by type: top card for single/straight,
// higher card of a pair, the triple's card, the quad or triple rank otherwise.
type Combo struct {
	Type  ComboType
	Cards []Card
	Key   Card
}

// StraightChecker reports whether five sorted cards form a straight, returning
// the run's high card.
type StraightChecker func(sorted []Card) (ok bool, high Card)

// SimpleStraight reports five consecutive ranks. No wrap: 2 is the top rank, so
// J-Q-K-A-2 is the highest straight and A-2-3-4-5 is not one. Input must be
// sorted ascending by Order.
func SimpleStraight(sorted []Card) (bool, Card) {
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Rank != sorted[i-1].Rank+1 {
			return false, Card{}
		}
	}
	return true, sorted[len(sorted)-1]
}

// Classify returns the combo formed by cards, or an error if they aren't a legal
// play. sc selects the straight variant.
func Classify(cards []Card, sc StraightChecker) (Combo, error) {
	if len(cards) == 0 {
		return Combo{}, ErrEmptyPlay
	}
	cs := append([]Card(nil), cards...)
	sortCards(cs)
	for i := 1; i < len(cs); i++ {
		if cs[i] == cs[i-1] {
			return Combo{}, ErrDuplicateCard
		}
	}
	switch len(cs) {
	case 1:
		return Combo{Type: Single, Cards: cs, Key: cs[0]}, nil
	case 2:
		if cs[0].Rank != cs[1].Rank {
			return Combo{}, ErrNotAPair
		}
		return Combo{Type: Pair, Cards: cs, Key: cs[1]}, nil
	case 3:
		if cs[0].Rank != cs[1].Rank || cs[1].Rank != cs[2].Rank {
			return Combo{}, ErrNotATriple
		}
		return Combo{Type: Triple, Cards: cs, Key: cs[2]}, nil
	case 5:
		return classifyFive(cs, sc)
	default:
		return Combo{}, ErrBadSize
	}
}

// classifyFive detects the five-card category, testing most-specific first.
func classifyFive(cs []Card, sc StraightChecker) (Combo, error) {
	flush := sameSuit(cs)
	straight, high := sc(cs)
	if straight && flush {
		return Combo{Type: StraightFlush, Cards: cs, Key: high}, nil
	}
	counts := rankCounts(cs) // counted once, shared by the checks below
	if r, ok := nOfAKind(counts, 4); ok {
		return Combo{Type: FourKind, Cards: cs, Key: Card{Rank: r, Suit: Spade}}, nil
	}
	if isFullHouse(counts) {
		return Combo{Type: FullHouse, Cards: cs, Key: Card{Rank: tripleRank(counts), Suit: Spade}}, nil
	}
	if flush {
		return Combo{Type: Flush, Cards: cs, Key: cs[4]}, nil
	}
	if straight {
		return Combo{Type: Straight, Cards: cs, Key: high}, nil
	}
	return Combo{}, ErrNoFiveCombo
}

// Beats reports whether a strictly beats b. Mismatched sizes never compare.
func (a Combo) Beats(b Combo) bool {
	if len(a.Cards) != len(b.Cards) {
		return false
	}
	switch len(a.Cards) {
	case 1:
		return a.Key.Order() > b.Key.Order()
	case 2, 3:
		// by rank; the suit tiebreak only matters for a multi-deck game.
		if a.Key.Rank != b.Key.Rank {
			return a.Key.Rank > b.Key.Rank
		}
		return a.Key.Suit > b.Key.Suit
	case 5:
		if a.Type != b.Type {
			return a.Type > b.Type // category strength
		}
		switch a.Type {
		case Flush:
			// rank-first: highest card down, then the top card's suit.
			for i := 4; i >= 0; i-- {
				if a.Cards[i].Rank != b.Cards[i].Rank {
					return a.Cards[i].Rank > b.Cards[i].Rank
				}
			}
			return a.Cards[4].Suit > b.Cards[4].Suit
		case FullHouse, FourKind:
			return a.Key.Rank > b.Key.Rank
		default: // Straight, StraightFlush
			if a.Key.Rank != b.Key.Rank {
				return a.Key.Rank > b.Key.Rank
			}
			return a.Key.Suit > b.Key.Suit
		}
	}
	return false
}

func sameSuit(cs []Card) bool {
	for i := 1; i < len(cs); i++ {
		if cs[i].Suit != cs[0].Suit {
			return false
		}
	}
	return true
}

func rankCounts(cs []Card) map[Rank]int {
	m := make(map[Rank]int, len(cs))
	for _, c := range cs {
		m[c.Rank]++
	}
	return m
}

func nOfAKind(counts map[Rank]int, n int) (Rank, bool) {
	for r, cnt := range counts {
		if cnt == n {
			return r, true
		}
	}
	return 0, false
}

func isFullHouse(counts map[Rank]int) bool {
	if len(counts) != 2 {
		return false
	}
	has3, has2 := false, false
	for _, cnt := range counts {
		switch cnt {
		case 3:
			has3 = true
		case 2:
			has2 = true
		}
	}
	return has3 && has2
}

func tripleRank(counts map[Rank]int) Rank {
	for r, cnt := range counts {
		if cnt == 3 {
			return r
		}
	}
	return 0
}
