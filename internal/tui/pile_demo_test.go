package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Avangelista/deuception/internal/game"
)

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripStyling removes SGR colour escapes and VS15 so a rendered frame can be
// matched by its plain composited glyphs.
func stripStyling(s string) string {
	return strings.ReplaceAll(sgrRe.ReplaceAllString(s, ""), vs15, "")
}

// pileFace is the composited left-border+rank+suit for a token, e.g. "6H" -> "│6♥".
func pileFace(tok string) string {
	c := card(tok)
	return "│" + string(c.Rank.Rune()) + string(c.Suit.Glyph())
}

// pileRowOf returns the topmost frame row showing tok's face, or -1.
func pileRowOf(frame, tok string) int {
	need := pileFace(tok)
	for r, line := range strings.Split(frame, "\n") {
		if strings.Contains(stripStyling(line), need) {
			return r
		}
	}
	return -1
}

// pileColOf returns the leftmost display column (rune index) of tok's face, or -1.
func pileColOf(frame, tok string) int {
	need := pileFace(tok)
	best := -1
	for _, line := range strings.Split(frame, "\n") {
		s := stripStyling(line)
		if i := strings.Index(s, need); i >= 0 {
			if col := len([]rune(s[:i])); best == -1 || col < best {
				best = col
			}
		}
	}
	return best
}

func card(s string) game.Card {
	c, err := game.ParseCard(s)
	if err != nil {
		panic(err)
	}
	return c
}
func cards(toks ...string) []game.Card {
	out := make([]game.Card, len(toks))
	for i, t := range toks {
		out[i] = card(t)
	}
	return out
}

// TestPileBoxMatchesDemo checks the rounded per-card pile against demo.txt exactly.
func TestPileBoxMatchesDemo(t *testing.T) {
	m := &Model{} // glyph mode (asciiSuits false)
	cases := []struct {
		name string
		cs   []game.Card
		want []string
	}{
		{"single", cards("2S"), []string{
			"╭────╮",
			"│2♠  │",
			"│    │",
			"╰────╯",
		}},
		{"pair", cards("4D", "4H"), []string{
			"╭──╭────╮",
			"│4♦│4♥  │",
			"│  │    │",
			"╰──╰────╯",
		}},
		{"straight", cards("5D", "6C", "7D", "8H", "9S"), []string{
			"╭──╭──╭──╭──╭────╮",
			"│5♦│6♣│7♦│8♥│9♠  │",
			"│  │  │  │  │    │",
			"╰──╰──╰──╰──╰────╯",
		}},
	}
	for _, tc := range cases {
		got := m.pileBoxLines(tc.cs)
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", tc.name,
				strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// TestPileFloatWidthInvariant checks the animated pile row is exactly w cells wide
// (colour escapes and VS15 are width-0) at every slide step, in both suit modes.
func TestPileFloatWidthInvariant(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		m := &Model{r: lipgloss.DefaultRenderer(), asciiSuits: ascii}
		m.st = newStyles(m.r)
		m.pileCur = cards("4D", "4H")
		m.pilePrev = cards("3D", "3H")
		m.pileDir = [2]int{1, 0}
		const w, h = 24, 6
		for step := 0; step <= pileSteps; step++ {
			m.pileStep = step
			for i, row := range strings.Split(m.pileFloat(w, h), "\n") {
				if got := lipgloss.Width(row); got != w {
					t.Errorf("ascii=%v step=%d row %d width=%d want %d\n%q",
						ascii, step, i, got, w, row)
				}
			}
		}
	}
}
