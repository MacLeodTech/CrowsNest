package history

import (
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	s, clock, _ := newStore(t, 0) // now is 2026-11-01 20:00 UTC
	at := func(day, hour int, month time.Month) { *clock = time.Date(2026, month, day, hour, 0, 0, 0, time.UTC) }
	at(29, 23, time.October)
	s.Record(KindJoin, "g", "ann", "") // before the window: only the part inside counts
	at(30, 23, time.October)
	s.Record(KindJoin, "g", "ann2", "")
	at(31, 1, time.October)
	s.Record(KindJoin, "g", "bob", "")
	s.Record(KindCrash, "g", "", "exit 1")
	at(31, 2, time.October)
	s.Record(KindLeave, "g", "ann2", "")
	at(31, 3, time.October)
	s.Record(KindLeave, "g", "bob", "")
	s.Record(KindStop, "g", "alex", "") // closes ann, who never left
	at(1, 19, time.November)
	s.Record(KindJoin, "g", "carol", "") // still aboard now
	s.Record(KindJoin, "other", "dan", "")
	at(1, 20, time.November)

	got := s.Stats(3, time.UTC, func(g string) bool { return g == "g" })
	if len(got) != 1 {
		t.Fatalf("stats = %+v", got)
	}
	g := got[0]
	// ann: Oct 30 00:00 to Oct 31 03:00 (the stop) = 27h; ann2 23:00 to 02:00 = 3h; bob 01:00-03:00 = 2h; carol 1h
	if g.PlayerSeconds != (27+3+2+1)*3600 {
		t.Errorf("player seconds = %d (%.1fh)", g.PlayerSeconds, float64(g.PlayerSeconds)/3600)
	}
	if g.Players != 4 || g.Visits != 3 || g.Crashes != 1 {
		t.Errorf("players %d visits %d crashes %d", g.Players, g.Visits, g.Crashes)
	}
	if g.Peak != 3 || g.PeakAt == "" { // ann, ann2 and bob between 01:00 and 02:00
		t.Errorf("peak %d at %q", g.Peak, g.PeakAt)
	}
	if len(g.Days) != 3 || g.Days[0].Date != "2026-10-30" || g.Days[2].Date != "2026-11-01" {
		t.Fatalf("days = %+v", g.Days)
	}
	if g.Days[2].PlayerSeconds != 3600 {
		t.Errorf("today = %ds, want carol's hour", g.Days[2].PlayerSeconds)
	}
	var sum int64
	for _, d := range g.Days {
		sum += d.PlayerSeconds
	}
	if sum != g.PlayerSeconds {
		t.Errorf("days add up to %d, total is %d", sum, g.PlayerSeconds)
	}
	if len(g.Top) == 0 || g.Top[0].Player != "ann" {
		t.Errorf("top = %+v", g.Top)
	}
	if all := s.Stats(3, time.UTC, nil); len(all) != 2 {
		t.Errorf("without a filter: %+v", all)
	}
	var none *Store
	if none.Stats(3, nil, nil) != nil {
		t.Error("a nil store returned stats")
	}
}
