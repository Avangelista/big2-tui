package game

import "errors"

// Move-validation and lifecycle errors; safe to surface directly to a player.
var (
	ErrGameNotActive    = errors.New("game not active")
	ErrNotYourTurn      = errors.New("not your turn")
	ErrNotAPair         = errors.New("not a valid pair")
	ErrNotATriple       = errors.New("not a valid triple")
	ErrBadSize          = errors.New("must play 1, 2, 3, or 5 cards")
	ErrNoFiveCombo      = errors.New("not a valid 5-card combination")
	ErrDuplicateCard    = errors.New("duplicate card in play")
	ErrDontOwnCards     = errors.New("you don't hold those cards")
	ErrMustPlayOpenCard = errors.New("must include the opening card")
	ErrWrongComboSize   = errors.New("must match the table count")
	ErrDoesNotBeat      = errors.New("does not beat the current play")
	ErrCannotPassOnLead = errors.New("you must lead, cannot pass")
	ErrBadPlayerCount   = errors.New("player count must be 2-4")
	ErrEmptyPlay        = errors.New("no cards selected")
)
