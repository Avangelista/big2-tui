// Package room hosts a single, in-memory Big 2 room. One actor goroutine owns
// all mutable state; sessions submit Commands and get per-viewer redacted
// snapshots. Nothing is persisted.
package room

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Avangelista/deuception/internal/game"
	"github.com/Avangelista/deuception/internal/protocol"
)

// Seat is one player position in the room.
type Seat struct {
	ID        string
	Prog      *tea.Program
	Connected bool
	Host      bool
	Score     int // cumulative penalty across hands (lower is better)
}

// Room is a single game room served to many connections.
type Room struct {
	cmds     chan Command
	maxSeats int
	minStart int
	rng      *mrand.Rand

	// actor-owned state (only touched inside run):
	seats []*Seat
	game  *game.GameState
	phase protocol.Phase
	rev   int // monotonic snapshot revision; lets clients drop out-of-order sends
}

// New starts a room actor. maxSeats caps the table, minStart is the fewest that can start.
func New(maxSeats, minStart int, rng *mrand.Rand) *Room {
	r := &Room{
		cmds:     make(chan Command, 64),
		maxSeats: maxSeats,
		minStart: minStart,
		rng:      rng,
		phase:    protocol.Waiting,
	}
	go r.run()
	return r
}

// Submit enqueues a command for the actor; safe from any goroutine.
func (r *Room) Submit(c Command) {
	defer func() { _ = recover() }() // tolerate submit after close
	r.cmds <- c
}

// NewID returns a random session/player identifier.
func NewID() string {
	b := make([]byte, 8)
	_, _ = crand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Room) run() {
	for c := range r.cmds {
		switch cmd := c.(type) {
		case JoinCmd:
			r.handleJoin(cmd)
		case StartCmd:
			r.handleStart(cmd)
		case PlayCmd:
			r.handlePlay(cmd)
		case PassCmd:
			r.handlePass(cmd)
		case NextHandCmd:
			r.handleNextHand(cmd)
		case DisconnectCmd:
			r.handleLeave(cmd.ID)
		case QuitCmd:
			r.handleLeave(cmd.ID)
		case queryCmd:
			if idx := r.seatIndexByID(cmd.id); idx >= 0 {
				cmd.reply <- r.snapshotFor(idx)
			} else {
				cmd.reply <- protocol.StateSnapshot{YouSeat: -1}
			}
		case closeCmd:
			for _, s := range r.seats {
				if s.Connected {
					safeSend(s.Prog, protocol.RoomClosedMsg{})
				}
			}
			close(cmd.done)
		}
	}
}

// closeCmd tells every connected player the room is shutting down.
type closeCmd struct{ done chan struct{} }

func (closeCmd) isCmd() {}

// Close notifies connected players and returns once the shutdown message is dispatched.
func (r *Room) Close() {
	done := make(chan struct{})
	r.Submit(closeCmd{done: done})
	<-done
}

// queryCmd requests a synchronous snapshot for a seat (used by tests).
type queryCmd struct {
	id    string
	reply chan protocol.StateSnapshot
}

func (queryCmd) isCmd() {}

// Query returns the redacted snapshot for id, serialized through the actor.
func (r *Room) Query(id string) protocol.StateSnapshot {
	reply := make(chan protocol.StateSnapshot, 1)
	r.Submit(queryCmd{id: id, reply: reply})
	return <-reply
}

func (r *Room) handleJoin(c JoinCmd) {
	// No reconnecting: a returning connection is a fresh id, turned away mid-game.
	// Its old seat stays in play as an auto-passing dropout.
	if r.phase != protocol.Waiting {
		safeSend(c.Prog, protocol.KickedMsg{Reason: "game already in progress"})
		return
	}
	if len(r.seats) >= r.maxSeats {
		safeSend(c.Prog, protocol.KickedMsg{Reason: "room is full"})
		return
	}
	// First to join is host (covers serve-only mode with no local host seat).
	isHost := c.Host || len(r.seats) == 0
	seat := &Seat{ID: c.ID, Prog: c.Prog, Connected: true, Host: isHost}
	r.seats = append(r.seats, seat)
	r.fanout()
}

func (r *Room) handleStart(c StartCmd) {
	s := r.seatByID(c.ID)
	if s == nil || !s.Host || r.phase != protocol.Waiting {
		return
	}
	if len(r.seats) < r.minStart {
		safeSend(s.Prog, protocol.ErrorMsg{Text: fmt.Sprintf("need at least %d players to start", r.minStart)})
		return
	}
	r.startGame()
	r.fanout()
}

func (r *Room) startGame() {
	r.game = game.NewGame(len(r.seats), game.SimpleStraight)
	if err := r.game.Deal(r.rng); err != nil {
		safeSendAll(r.seats, protocol.ErrorMsg{Text: "failed to deal: " + err.Error()})
		return
	}
	r.phase = protocol.InGame
}

func (r *Room) handlePlay(c PlayCmd) {
	if r.phase != protocol.InGame {
		return
	}
	idx := r.seatIndexByID(c.ID)
	if idx < 0 {
		return
	}
	evs, err := r.game.Play(game.Seat(idx), c.Cards)
	if err != nil {
		safeSend(r.seats[idx].Prog, protocol.ErrorMsg{Text: err.Error()})
		return
	}
	r.applyEvents(evs)
	r.autoAdvanceForDisconnected() // don't stall if the turn lands on a dropped player
	r.fanout()
}

func (r *Room) handlePass(c PassCmd) {
	if r.phase != protocol.InGame {
		return
	}
	idx := r.seatIndexByID(c.ID)
	if idx < 0 {
		return
	}
	evs, err := r.game.Pass(game.Seat(idx))
	if err != nil {
		safeSend(r.seats[idx].Prog, protocol.ErrorMsg{Text: err.Error()})
		return
	}
	r.applyEvents(evs)
	r.autoAdvanceForDisconnected() // don't stall if the turn lands on a dropped player
	r.fanout()
}

func (r *Room) handleNextHand(c NextHandCmd) {
	s := r.seatByID(c.ID)
	if s == nil || !s.Host || r.phase != protocol.Finished {
		return
	}
	r.game = game.NewGame(len(r.seats), game.SimpleStraight)
	if err := r.game.Deal(r.rng); err != nil {
		return
	}
	r.phase = protocol.InGame
	r.autoAdvanceForDisconnected() // a carried-over dropped player may be the leader
	r.fanout()
}

func (r *Room) handleLeave(id string) {
	idx := r.seatIndexByID(id)
	if idx < 0 {
		return
	}
	seat := r.seats[idx]
	if r.phase == protocol.Waiting {
		r.seats = append(r.seats[:idx], r.seats[idx+1:]...)
		r.fanout()
		return
	}
	seat.Connected = false
	r.autoAdvanceForDisconnected()
	r.fanout()
}

// autoAdvanceForDisconnected keeps play moving on a disconnected seat's turn:
// pass if it can, else lead its lowest card.
func (r *Room) autoAdvanceForDisconnected() {
	guard := 0
	for r.phase == protocol.InGame && !r.seats[r.game.Turn].Connected {
		guard++
		if guard > 500 {
			return
		}
		turn := r.game.Turn
		var evs []game.Event
		var err error
		if r.game.Table == nil {
			hand := r.game.Hands[turn]
			if len(hand) == 0 {
				return
			}
			evs, err = r.game.Play(turn, []game.Card{hand[0]}) // hand is sorted ascending
		} else {
			evs, err = r.game.Pass(turn)
		}
		if err != nil {
			return
		}
		r.applyEvents(evs)
	}
}

func (r *Room) applyEvents(evs []game.Event) {
	// The table view carries all live state; only the end-of-hand event needs
	// handling here.
	for _, e := range evs {
		if ev, ok := e.(game.GameWonEvent); ok {
			r.handleGameWon(ev)
		}
	}
}

func (r *Room) handleGameWon(ev game.GameWonEvent) {
	scores := r.game.HandScores()
	for i := range r.seats {
		r.seats[i].Score += scores[i]
	}
	r.phase = protocol.Finished
	_ = ev
}

// fanout pushes a per-viewer redacted snapshot to every connected seat, bumping
// rev so clients can drop out-of-order sends.
func (r *Room) fanout() {
	r.rev++
	for i, s := range r.seats {
		if !s.Connected || s.Prog == nil {
			continue
		}
		safeSend(s.Prog, protocol.StateSnapshotMsg{Snap: r.snapshotFor(i)})
	}
}

// snapshotFor builds the redacted view for viewer. Redaction choke point:
// opponents' cards never leave here, only counts.
func (r *Room) snapshotFor(viewer int) protocol.StateSnapshot {
	players := make([]protocol.PlayerView, len(r.seats))
	for i, s := range r.seats {
		pv := protocol.PlayerView{
			Seat:      i,
			Connected: s.Connected,
			IsYou:     i == viewer,
			IsHost:    s.Host,
			Score:     s.Score,
		}
		if r.game != nil {
			pv.CardCount = len(r.game.Hands[i])
			pv.IsTurn = r.phase == protocol.InGame && int(r.game.Turn) == i
			// passing is locked out, so this stays set for the whole trick
			pv.Passed = r.phase == protocol.InGame && r.game.Passed[i]
		}
		players[i] = pv
	}
	snap := protocol.StateSnapshot{
		Phase:    r.phase,
		Rev:      r.rev,
		YouSeat:  viewer,
		IsHost:   r.seats[viewer].Host,
		MaxSeats: r.maxSeats,
		MinStart: r.minStart,
		Players:  players,
		Turn:     -1,
		TableBy:  -1,
		Winner:   -1,
	}
	if r.game != nil {
		snap.YourHand = append([]game.Card(nil), r.game.Hands[viewer]...)
		if r.game.Table != nil {
			snap.Table = append([]game.Card(nil), r.game.Table.Cards...)
			snap.TableBy = int(r.game.Leader) // Leader owns the current Table combo
		}
		snap.Turn = int(r.game.Turn)
		snap.Winner = int(r.game.Winner)
	}
	return snap
}

func (r *Room) seatByID(id string) *Seat {
	for _, s := range r.seats {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (r *Room) seatIndexByID(id string) int {
	for i, s := range r.seats {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// safeSend sends msg from a fresh goroutine, tolerating a torn-down program.
func safeSend(p *tea.Program, msg tea.Msg) {
	if p == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		p.Send(msg)
	}()
}

func safeSendAll(seats []*Seat, msg tea.Msg) {
	for _, s := range seats {
		if s.Connected {
			safeSend(s.Prog, msg)
		}
	}
}
