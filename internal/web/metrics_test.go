package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/auth"
)

type fakeCrashes struct{}

func (fakeCrashes) Total(id string) uint64 {
	if id == "windrose" {
		return 2
	}
	return 0
}
func (fakeCrashes) Looping(id string) bool { return id == "windrose" }

func TestMetrics_TokenGatedAndExposesGames(t *testing.T) {
	h := newHarness(t, nil, "windrose")
	h.App.HostPaths = []string{t.TempDir()}

	// No token configured: the route does not exist.
	rec := httptest.NewRecorder()
	h.mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code == http.StatusOK || rec.Code == http.StatusUnauthorized {
		t.Fatalf("/metrics without a token configured = %d, want it absent", rec.Code)
	}

	h.App.MetricsToken = "s3cret"
	h.App.Crashes = fakeCrashes{}
	get := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.mux().ServeHTTP(rec, req)
		return rec
	}
	for _, bad := range []string{"", "Bearer nope", "s3cret", "Basic s3cret"} {
		if rec := get(bad); rec.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q = %d, want 401", bad, rec.Code)
		}
	}
	rec = get("Bearer s3cret")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("authorised = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		"# TYPE crowsnest_game_up gauge", `crowsnest_game_up{game="windrose"} 0`,
		`crowsnest_game_crashes_total{game="windrose"} 2`, `crowsnest_game_crash_looping{game="windrose"} 1`,
		"crowsnest_host_memory_available_bytes ", "crowsnest_host_disk_free_bytes{path=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q in:\n%s", want, body)
		}
	}
}

func TestListGames_FlagsCrashLooping(t *testing.T) {
	h := newHarness(t, nil, "windrose", "other")
	h.App.Crashes = fakeCrashes{}
	rec := httptest.NewRecorder()
	h.mux().ServeHTTP(rec, h.req(http.MethodGet, "/api/games", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/games = %d", rec.Code)
	}
	var got []struct {
		ID           string `json:"id"`
		CrashLooping bool   `json:"crash_looping"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if want := g.ID == "windrose"; g.CrashLooping != want {
			t.Errorf("%s crash_looping = %v, want %v", g.ID, g.CrashLooping, want)
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %d games, want 2", len(got))
	}
}

func TestMetrics_WrongTokensAreThrottled(t *testing.T) {
	h := newHarness(t, nil, "windrose")
	h.App.HostPaths = []string{t.TempDir()}
	h.App.MetricsToken = "s3cret"
	h.App.MetricsLimiter = auth.NewLimiter(3, time.Minute)
	get := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.mux().ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if got := get("wrong"); got != http.StatusUnauthorized {
			t.Fatalf("wrong token %d = %d", i, got)
		}
	}
	if got := get("s3cret"); got != http.StatusTooManyRequests {
		t.Errorf("right token after 3 failures = %d, want 429", got)
	}
}
