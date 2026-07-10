package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// styles is deliberately plain - no colour, no bold - so the board reads like
// ordinary terminal output.
type styles struct {
	faint lipgloss.Style
	turn  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	// emphasis comes from layout (e.g. [brackets]), not colour.
	plain := r.NewStyle()
	return styles{faint: plain, turn: plain}
}
