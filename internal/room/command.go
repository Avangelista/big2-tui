package room

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Avangelista/deuception/internal/game"
)

// Command is a message submitted to a room's actor goroutine; all state mutation
// happens there.
type Command interface{ isCmd() }

// JoinCmd asks to seat a connection. Prog pushes state back to it; Host marks the
// local host.
type JoinCmd struct {
	ID   string
	Prog *tea.Program
	Host bool
}

// StartCmd (host only) starts the game if enough players are seated.
type StartCmd struct{ ID string }

// PlayCmd attempts to play the given cards for the sender's seat.
type PlayCmd struct {
	ID    string
	Cards []game.Card
}

// PassCmd passes for the sender's seat.
type PassCmd struct{ ID string }

// NextHandCmd (host only) deals a fresh hand after one finishes, keeping scores.
type NextHandCmd struct{ ID string }

// DisconnectCmd marks a seat's connection gone (SSH context cancelled).
type DisconnectCmd struct{ ID string }

// QuitCmd is a graceful leave (player pressed quit).
type QuitCmd struct{ ID string }

func (JoinCmd) isCmd()       {}
func (StartCmd) isCmd()      {}
func (PlayCmd) isCmd()       {}
func (PassCmd) isCmd()       {}
func (NextHandCmd) isCmd()   {}
func (DisconnectCmd) isCmd() {}
func (QuitCmd) isCmd()       {}
