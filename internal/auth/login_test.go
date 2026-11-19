package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/mfa"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	hash, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	return &Service{
		Users:        Users{"alice": {Hash: hash}},
		Sessions:     NewSessionManager([]byte("test-secret"), time.Hour),
		LoginLimiter: NewLimiter(100, time.Minute),
	}
}

func doLogin(t *testing.T, svc *Service, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(loginRequest{Username: username, Password: password})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.ServeLogin(rec, req)
	return rec
}

func TestServeLogin_Success(t *testing.T) {
	svc := newTestService(t)
	rec := doLogin(t, svc, "Alice", "s3cret-password")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName || cookies[0].Value == "" {
		t.Fatalf("cookies = %+v, want one non-empty %s cookie", cookies, SessionCookieName)
	}

	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.CSRFToken == "" {
		t.Fatal("response CSRF token is empty")
	}

	session, err := svc.Sessions.Decode(cookies[0].Value)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if session.Username != "alice" || session.CSRFToken != resp.CSRFToken {
		t.Fatalf("session = %+v, want username alice matching returned csrf token", session)
	}
}

func TestServeLogin_WrongPassword(t *testing.T) {
	svc := newTestService(t)
	rec := doLogin(t, svc, "alice", "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServeLogin_UnknownUser(t *testing.T) {
	svc := newTestService(t)
	rec := doLogin(t, svc, "ghost", "whatever")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServeLogin_RateLimited(t *testing.T) {
	svc := newTestService(t)
	svc.LoginLimiter = NewLimiter(1, time.Minute)

	first := doLogin(t, svc, "alice", "wrong-password")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first attempt status = %d, want %d", first.Code, http.StatusUnauthorized)
	}
	second := doLogin(t, svc, "alice", "s3cret-password")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second attempt status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestServeLogin_BodyTooLarge(t *testing.T) {
	svc := newTestService(t)

	oversized := bytes.Repeat([]byte("a"), maxLoginBodyBytes+1)
	body, _ := json.Marshal(loginRequest{Username: "alice", Password: string(oversized)})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc.ServeLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for an oversized body", rec.Code, http.StatusBadRequest)
	}
}

func TestServeLogout_ClearsCookie(t *testing.T) {
	svc := newTestService(t)
	rec := httptest.NewRecorder()
	svc.ServeLogout(rec, httptest.NewRequest(http.MethodPost, "/logout", nil))

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName {
		t.Fatalf("cookies = %+v, want one %s cookie", cookies, SessionCookieName)
	}
	if cookies[0].MaxAge >= 0 {
		t.Fatalf("logout cookie MaxAge = %d, want negative (delete)", cookies[0].MaxAge)
	}
}

type fakeMFA struct {
	code     string
	enabled  bool
	attempts int
}

func (f *fakeMFA) Enabled(string) bool     { return f.enabled }
func (f *fakeMFA) RecoveryLeft(string) int { return 3 }
func (f *fakeMFA) Verify(_, code string) (bool, error) {
	f.attempts++
	if code == f.code {
		return false, nil
	}
	return false, mfa.ErrBadCode
}

func TestLoginWithSecondFactor(t *testing.T) {
	hash, _ := HashPassword("correct horse")
	svc := &Service{Users: Users{"ann": {Hash: hash}}, Sessions: NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour)}
	svc.Sessions.Users = svc.Users
	f := &fakeMFA{code: "123456", enabled: true}
	svc.MFA = f

	post := func(h http.HandlerFunc, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	rec := post(svc.ServeLogin, `{"username":"ann","password":"correct horse"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "mfa_required") {
		t.Fatalf("password step = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			t.Fatal("a session was issued before the second factor")
		}
	}
	var pending *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == mfaCookieName {
			pending = c
		}
	}
	if pending == nil {
		t.Fatal("no pending cookie")
	}
	if rec := post(svc.ServeLoginMFA, `{"code":"123456"}`); rec.Code != 401 {
		t.Errorf("code without the pending cookie = %d, want 401", rec.Code)
	}
	if rec := post(svc.ServeLoginMFA, `{"code":"000000"}`, pending); rec.Code != 401 {
		t.Errorf("wrong code = %d, want 401", rec.Code)
	}
	rec = post(svc.ServeLoginMFA, `{"code":"123456"}`, pending)
	if rec.Code != 200 {
		t.Fatalf("right code = %d %s", rec.Code, rec.Body.String())
	}
	var got bool
	for _, c := range rec.Result().Cookies() {
		got = got || (c.Name == SessionCookieName && c.Value != "")
	}
	if !got {
		t.Error("no session after the second factor")
	}
	// A user without a second factor signs in in one step.
	f.enabled = false
	if rec := post(svc.ServeLogin, `{"username":"ann","password":"correct horse"}`); !strings.Contains(rec.Body.String(), "csrf_token") {
		t.Errorf("no-MFA login = %s", rec.Body.String())
	}
}

func TestLoginThrottle_OnlyFailuresCountAndPerUser(t *testing.T) {
	svc := newTestService(t)
	svc.LoginLimiter = NewLimiter(3, time.Minute)
	svc.UserLimiter = NewLimiter(3, time.Minute)
	// Correct sign-ins never use the allowance up.
	for i := 0; i < 10; i++ {
		if rec := doLogin(t, svc, "alice", "s3cret-password"); rec.Code != http.StatusOK {
			t.Fatalf("sign-in %d = %d", i, rec.Code)
		}
	}
	// Three wrong guesses block the address, even for the right password.
	for i := 0; i < 3; i++ {
		if rec := doLogin(t, svc, "alice", "nope"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d", i, rec.Code)
		}
	}
	if rec := doLogin(t, svc, "alice", "s3cret-password"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after 3 failures = %d, want 429", rec.Code)
	}

	// Guesses from many addresses still add up against the one username.
	svc = newTestService(t)
	svc.LoginLimiter = NewLimiter(100, time.Minute)
	svc.UserLimiter = NewLimiter(3, time.Minute)
	from := func(ip, pw string) int {
		body, _ := json.Marshal(loginRequest{Username: "alice", Password: pw})
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		svc.ServeLogin(rec, req)
		return rec.Code
	}
	for i, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		if got := from(ip, "nope"); got != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d", i, got)
		}
	}
	if got := from("192.0.2.4", "s3cret-password"); got != http.StatusTooManyRequests {
		t.Fatalf("a fourth address after 3 failures = %d, want 429", got)
	}
	// Another user is unaffected.
	if got := doLogin(t, svc, "bob", "x"); got != nil && got.Code == http.StatusTooManyRequests {
		t.Error("another username was throttled")
	}
}

func TestLogin_UpgradesAWeakHash(t *testing.T) {
	old := PasswordCost
	defer func() { PasswordCost = old }()
	PasswordCost = 4
	path := filepath.Join(t.TempDir(), "users")
	weak, _ := HashPassword("s3cret-password")
	if err := SetUserHash(path, "alice", weak); err != nil {
		t.Fatal(err)
	}
	store, err := NewUserStore(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: store, Sessions: NewSessionManager([]byte("test-secret"), time.Hour)}
	PasswordCost = 5 // the cost goes up after the hash was made
	if rec := doLogin(t, svc, "alice", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d", rec.Code)
	}
	if r, _ := store.Lookup("alice"); r.Hash != weak {
		t.Fatal("hash changed on a failed sign-in")
	}
	if rec := doLogin(t, svc, "alice", "s3cret-password"); rec.Code != http.StatusOK {
		t.Fatalf("sign-in = %d", rec.Code)
	}
	r, _ := store.Lookup("alice")
	if r.Hash == weak || HashNeedsUpgrade(r.Hash) || !VerifyPassword(r.Hash, "s3cret-password") {
		t.Fatalf("hash not upgraded: %q", r.Hash)
	}
}

func TestLogin_RequiresJSONContentType(t *testing.T) {
	svc := newTestService(t)
	body, _ := json.Marshal(loginRequest{Username: "alice", Password: "s3cret-password"})
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data"} {
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		svc.ServeLogin(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q = %d, want 415", ct, rec.Code)
		}
	}
	// A charset parameter is still JSON.
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	svc.ServeLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("json with a charset = %d, want 200", rec.Code)
	}
}

// A stranger failing on purpose must not lock the real person out of their usual address.
func TestLoginThrottle_KnownAddressSurvivesUserLockout(t *testing.T) {
	svc := newTestService(t)
	svc.LoginLimiter = NewLimiter(100, time.Minute)
	svc.UserLimiter = NewLimiter(3, time.Minute)
	from := func(ip, pw string) int {
		body, _ := json.Marshal(loginRequest{Username: "alice", Password: pw})
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		svc.ServeLogin(rec, req)
		return rec.Code
	}
	if got := from("192.0.2.9", "s3cret-password"); got != http.StatusOK {
		t.Fatalf("first sign-in = %d", got)
	}
	for i, ip := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		if got := from(ip, "nope"); got != http.StatusUnauthorized {
			t.Fatalf("attack %d = %d", i, got)
		}
	}
	if got := from("198.51.100.4", "s3cret-password"); got != http.StatusTooManyRequests {
		t.Errorf("a stranger's address during the attack = %d, want 429", got)
	}
	if got := from("192.0.2.9", "s3cret-password"); got != http.StatusOK {
		t.Errorf("the user's own address during the attack = %d, want 200", got)
	}
}

func TestLimiterHit_IsBounded(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 1000; i++ {
		l.Hit("k")
	}
	if n := len(l.hits["k"]); n != 3 {
		t.Errorf("kept %d hits, want 3", n)
	}
	if !l.Blocked("k") {
		t.Error("key should still be blocked")
	}
}

func TestKnownAddrsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-addrs.json")
	var a Service
	a.LoadKnownAddrs(path)
	a.known.add("alex", "203.0.113.7")
	var b Service // the panel restarted
	b.LoadKnownAddrs(path)
	if !b.known.has("alex", "203.0.113.7") {
		t.Error("a known address was forgotten across a restart")
	}
	if b.known.has("alex", "198.51.100.1") || b.known.has("sam", "203.0.113.7") {
		t.Error("an address became known for someone it was not")
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file: %v %v", fi, err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var c Service
	c.LoadKnownAddrs(path) // unreadable: starts empty, no panic
	if c.known.has("alex", "203.0.113.7") {
		t.Error("garbage file produced an exemption")
	}
}
