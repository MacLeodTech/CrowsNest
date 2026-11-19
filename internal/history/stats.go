package history

import (
	"sort"
	"time"
)

// GameStats is what a game's activity added up to over a window.
type GameStats struct {
	Game          string  `json:"game"`
	PlayerSeconds int64   `json:"player_seconds"` // time all players spent aboard, summed
	Visits        int     `json:"visits"`         // joins in the window
	Players       int     `json:"players"`        // different players seen in the window
	Peak          int     `json:"peak"`           // most aboard at once
	PeakAt        string  `json:"peak_at,omitempty"`
	Crashes       int     `json:"crashes"`
	Days          []Day   `json:"days"` // oldest first, one per local day in the window
	Top           []Share `json:"top"`  // players by time aboard, longest first (at most five)
}

// Day is the time players spent aboard on one local day.
type Day struct {
	Date          string `json:"date"` // 2006-01-02
	PlayerSeconds int64  `json:"player_seconds"`
}

// Share is one player's time aboard.
type Share struct {
	Player  string `json:"player"`
	Seconds int64  `json:"seconds"`
}

type span struct {
	game, player string
	from, to     time.Time
}

// spans pairs joins with leaves the way Playtime does (a stop closes every open visit of the game; a visit with no
// end runs to now). Caller holds s.mu.
func (s *Store) spans(now time.Time) []span {
	open := map[ptKey]time.Time{}
	var out []span
	for _, e := range s.events {
		switch e.Kind {
		case KindJoin:
			if _, ok := open[ptKey{e.Game, e.Actor}]; !ok {
				open[ptKey{e.Game, e.Actor}] = e.Time
			}
		case KindLeave:
			k := ptKey{e.Game, e.Actor}
			if st, ok := open[k]; ok {
				out = append(out, span{k.game, k.player, st, e.Time})
				delete(open, k)
			}
		case KindStop, KindAutoStop:
			for k, st := range open {
				if k.game == e.Game {
					out = append(out, span{k.game, k.player, st, e.Time})
					delete(open, k)
				}
			}
		}
	}
	for k, st := range open {
		out = append(out, span{k.game, k.player, st, now})
	}
	return out
}

// Stats adds up each game's activity over the last days days (counted in loc's calendar days, today included).
// Games with no activity in the window are left out. allow, when set, limits which games are returned.
func (s *Store) Stats(days int, loc *time.Location, allow func(game string) bool) []GameStats {
	if s == nil || days < 1 {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}
	now := s.now()
	today := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	since := today.AddDate(0, 0, -(days - 1))

	s.mu.Lock()
	spans := s.spans(now)
	crashes := map[string]int{}
	joins := map[string]int{}
	for _, e := range s.events {
		if e.Time.Before(since) {
			continue
		}
		switch e.Kind {
		case KindCrash:
			crashes[e.Game]++
		case KindJoin:
			joins[e.Game]++
		}
	}
	s.mu.Unlock()

	type acc struct {
		perPlayer map[string]time.Duration
		perDay    map[string]time.Duration
		edges     []edge
	}
	by := map[string]*acc{}
	get := func(g string) *acc {
		if by[g] == nil {
			by[g] = &acc{perPlayer: map[string]time.Duration{}, perDay: map[string]time.Duration{}}
		}
		return by[g]
	}
	for _, sp := range spans {
		from, to := sp.from, sp.to
		if from.Before(since) {
			from = since
		}
		if !to.After(from) {
			continue
		}
		a := get(sp.game)
		a.perPlayer[sp.player] += to.Sub(from)
		a.edges = append(a.edges, edge{from, 1}, edge{to, -1})
		// split across local midnights
		for t := from; t.Before(to); {
			d := t.In(loc)
			next := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, loc)
			if next.After(to) {
				next = to
			}
			a.perDay[d.Format("2006-01-02")] += next.Sub(t)
			t = next
		}
	}
	for g := range crashes {
		get(g)
	}
	var out []GameStats
	for g, a := range by {
		if allow != nil && !allow(g) {
			continue
		}
		gs := GameStats{Game: g, Visits: joins[g], Players: len(a.perPlayer), Crashes: crashes[g]}
		for _, d := range a.perPlayer {
			gs.PlayerSeconds += int64(d.Seconds())
		}
		sort.Slice(a.edges, func(i, j int) bool {
			if !a.edges[i].t.Equal(a.edges[j].t) {
				return a.edges[i].t.Before(a.edges[j].t)
			}
			return a.edges[i].d < a.edges[j].d // a leave at the same instant as a join comes first
		})
		cur := 0
		for _, e := range a.edges {
			cur += e.d
			if cur > gs.Peak {
				gs.Peak, gs.PeakAt = cur, e.t.UTC().Format(time.RFC3339)
			}
		}
		for i := 0; i < days; i++ {
			key := since.AddDate(0, 0, i).Format("2006-01-02")
			gs.Days = append(gs.Days, Day{Date: key, PlayerSeconds: int64(a.perDay[key].Seconds())})
		}
		for p, d := range a.perPlayer {
			gs.Top = append(gs.Top, Share{Player: p, Seconds: int64(d.Seconds())})
		}
		sort.Slice(gs.Top, func(i, j int) bool {
			if gs.Top[i].Seconds != gs.Top[j].Seconds {
				return gs.Top[i].Seconds > gs.Top[j].Seconds
			}
			return gs.Top[i].Player < gs.Top[j].Player
		})
		if len(gs.Top) > 5 {
			gs.Top = gs.Top[:5]
		}
		out = append(out, gs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PlayerSeconds != out[j].PlayerSeconds {
			return out[i].PlayerSeconds > out[j].PlayerSeconds
		}
		return out[i].Game < out[j].Game
	})
	return out
}

type edge struct {
	t time.Time
	d int
}
