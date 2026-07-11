package room

import (
	mrand "math/rand"
	"testing"
	"time"

	"github.com/Avangelista/deuception/internal/game"
	"github.com/Avangelista/deuception/internal/protocol"
)

func countBots(s protocol.StateSnapshot) int {
	n := 0
	for _, p := range s.Players {
		if p.IsBot {
			n++
		}
	}
	return n
}

// TestAddAndRemoveBot covers the host-only add/remove and that a fresh bot gets a
// unique in-range letter.
func TestAddAndRemoveBot(t *testing.T) {
	r := New(4, 2, mrand.New(mrand.NewSource(1)))
	host := NewID()
	r.Submit(JoinCmd{ID: host, Host: true})
	r.Submit(AddBotCmd{ID: host, Level: 8})

	snap := r.Query(host)
	if len(snap.Players) != 2 {
		t.Fatalf("players after add = %d, want 2", len(snap.Players))
	}
	b := snap.Players[1]
	if !b.IsBot || b.BotLevel != 8 {
		t.Fatalf("seat 1: IsBot=%v level=%d, want bot level 8", b.IsBot, b.BotLevel)
	}
	if b.Letter == snap.Players[0].Letter || b.Letter < 'A' || b.Letter > 'Z' {
		t.Fatalf("bot letter %q collides or is out of range", b.Letter)
	}

	// A non-host may neither add nor remove bots.
	p2 := NewID()
	r.Submit(JoinCmd{ID: p2})
	r.Submit(AddBotCmd{ID: p2, Level: 3})
	if n := len(r.Query(host).Players); n != 3 {
		t.Fatalf("non-host add changed the table (players=%d, want 3)", n)
	}
	r.Submit(RemoveBotCmd{ID: p2})
	if bots := countBots(r.Query(host)); bots != 1 {
		t.Fatalf("non-host remove changed the table (bots=%d, want 1)", bots)
	}

	// The host removes the bot.
	r.Submit(RemoveBotCmd{ID: host})
	if bots := countBots(r.Query(host)); bots != 0 {
		t.Fatalf("bots after host remove = %d, want 0", bots)
	}
}

// TestLetterClaimAndReject: letters default unique, a claim takes a free letter,
// and one human cannot steal another human's letter.
func TestLetterClaimAndReject(t *testing.T) {
	r := New(4, 2, mrand.New(mrand.NewSource(2)))
	host := NewID()
	r.Submit(JoinCmd{ID: host, Host: true})
	p2 := NewID()
	r.Submit(JoinCmd{ID: p2})

	snap := r.Query(host)
	if snap.Players[0].Letter == snap.Players[1].Letter {
		t.Fatalf("default letters collide (%c)", snap.Players[0].Letter)
	}

	r.Submit(SetLetterCmd{ID: host, Letter: 'z'}) // lower-case is upcased server-side
	if got := r.Query(host).Players[0].Letter; got != 'Z' {
		t.Fatalf("host letter = %c, want Z", got)
	}

	before := r.Query(p2).Players[1].Letter
	r.Submit(SetLetterCmd{ID: p2, Letter: 'Z'}) // held by a human: rejected
	after := r.Query(p2).Players[1].Letter
	if after == 'Z' || after != before {
		t.Fatalf("rejected claim changed p2 letter to %c (was %c)", after, before)
	}
}

// TestHumanBumpsBotLetter: a human always wins a contested letter; a bot holding
// it is bumped to a new free letter.
func TestHumanBumpsBotLetter(t *testing.T) {
	r := New(4, 2, mrand.New(mrand.NewSource(3)))
	host := NewID()
	r.Submit(JoinCmd{ID: host, Host: true})
	r.Submit(AddBotCmd{ID: host, Level: 5})

	snap := r.Query(host)
	if !snap.Players[1].IsBot {
		t.Fatal("seat 1 is not the bot")
	}
	botLetter := snap.Players[1].Letter

	r.Submit(SetLetterCmd{ID: host, Letter: botLetter})
	snap = r.Query(host)
	if snap.Players[0].Letter != botLetter {
		t.Fatalf("host did not take the bot's letter %c (got %c)", botLetter, snap.Players[0].Letter)
	}
	if snap.Players[1].Letter == botLetter {
		t.Fatal("bot kept its letter after being bumped")
	}
	if snap.Players[0].Letter == snap.Players[1].Letter {
		t.Fatal("letters collide after the bump")
	}
}

// humanDumbMove submits a guaranteed-legal move for a human seat: lead the lowest
// card, beat a single if possible, otherwise pass (mirrors playOutHand).
func humanDumbMove(r *Room, id string, snap protocol.StateSnapshot) {
	hand := snap.YourHand
	if len(hand) == 0 {
		return
	}
	if len(snap.Table) == 0 {
		r.Submit(PlayCmd{ID: id, Cards: []game.Card{hand[0]}})
		return
	}
	if len(snap.Table) == 1 {
		tbl, _ := game.Classify(snap.Table, game.SimpleStraight)
		for _, c := range hand {
			cc, _ := game.Classify([]game.Card{c}, game.SimpleStraight)
			if cc.Beats(tbl) {
				r.Submit(PlayCmd{ID: id, Cards: []game.Card{c}})
				return
			}
		}
	}
	r.Submit(PassCmd{ID: id})
}

// TestBotPlaysThroughActor drives a whole hand with one human and three bots. The
// bots resolve their own turns via the scheduler (botDelay 0 for speed); the game
// must reach a winner without stalling. Run under -race to exercise the scheduler
// goroutines against the single-owner actor.
func TestBotPlaysThroughActor(t *testing.T) {
	r := New(4, 3, mrand.New(mrand.NewSource(5)))
	r.botDelay = 0 // no artificial think time in tests

	host := NewID()
	r.Submit(JoinCmd{ID: host, Host: true})
	for i := 0; i < 3; i++ {
		r.Submit(AddBotCmd{ID: host, Level: 6})
	}
	r.Submit(StartCmd{ID: host})

	deadline := time.Now().Add(15 * time.Second)
	for {
		snap := r.Query(host)
		if snap.Phase == protocol.Finished {
			if snap.Winner < 0 {
				t.Fatal("game finished with no winner")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("bot-driven game stalled; phase=%v turn=%d", snap.Phase, snap.Turn)
		}
		if snap.Phase == protocol.InGame && snap.Turn == snap.YouSeat {
			humanDumbMove(r, host, snap)
		} else {
			time.Sleep(time.Millisecond) // let scheduled bot moves land
		}
	}
}
