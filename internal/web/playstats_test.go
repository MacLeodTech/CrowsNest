package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/auth"
	"github.com/macleodtech/crowsnest/internal/history"
)

func TestPlayStats_OnlyVisibleGames(t *testing.T) {
	h := newHarness(t, nil, "windrose", "other")
	store, err := history.Open(filepath.Join(t.TempDir(), "events.jsonl"), 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	store.Now = func() time.Time { return now.Add(-2 * time.Hour) }
	store.Record(history.KindJoin, "windrose", "ann", "")
	store.Record(history.KindJoin, "nowhere", "bob", "") // a game this user cannot see
	store.Now = func() time.Time { return now.Add(-time.Hour) }
	store.Record(history.KindLeave, "windrose", "ann", "")
	store.Record(history.KindLeave, "nowhere", "bob", "")
	store.Now = func() time.Time { return now }
	h.App.History = store
	h.App.TimeZone = "UTC"
	h.App.Auth.Sessions.Users = auth.Users{"tester": {Role: auth.RolePlayer, Games: []string{"windrose"}}}

	rec := httptest.NewRecorder()
	h.mux().ServeHTTP(rec, h.req(http.MethodGet, "/api/play-stats?days=7", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		Days  int                 `json:"days"`
		Games []history.GameStats `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Days != 7 || len(out.Games) != 1 || out.Games[0].Game != "windrose" || out.Games[0].PlayerSeconds != 3600 {
		t.Errorf("stats = %+v", out)
	}
	if len(out.Games[0].Days) != 7 {
		t.Errorf("days = %d, want 7", len(out.Games[0].Days))
	}
}
