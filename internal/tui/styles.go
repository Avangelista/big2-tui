package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// styles keeps the board plain - the turn cue is still the [brackets], opponent
// backs stay unfaded - and adds colour only to the cards: red suits, and the
// cursor/selected accents on the viewer's own hand.
type styles struct {
	faint    lipgloss.Style
	turn     lipgloss.Style
	suitRed  lipgloss.Style // hearts, diamonds
	cursor   lipgloss.Style // cursor card border + its "*"
	selected lipgloss.Style // selected card border
}

func newStyles(r *lipgloss.Renderer) styles {
	plain := r.NewStyle()
	return styles{
		faint:    plain,
		turn:     plain, // turn emphasis stays the [brackets], not colour
		suitRed:  r.NewStyle().Foreground(lipgloss.Color("1")),
		cursor:   r.NewStyle().Foreground(lipgloss.Color("6")), // cyan
		selected: r.NewStyle().Foreground(lipgloss.Color("3")), // yellow
	}
}
