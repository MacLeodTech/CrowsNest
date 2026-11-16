package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/macleodtech/crowsnest/internal/audit"
)

// handleUserSessions lists another person's signed-in devices (admins only), newest first as the registry keeps them.
func (a *App) handleUserSessions(w http.ResponseWriter, r *http.Request) {
	reg := a.Auth.Sessions.Registry
	if reg == nil {
		http.Error(w, "this server does not keep a session list", http.StatusNotFound)
		return
	}
	name := strings.ToLower(r.PathValue("name"))
	out := []profileSession{}
	for _, rec := range reg.List(name) {
		out = append(out, profileSession{ID: rec.ID, Device: describeDevice(rec.UserAgent), IP: rec.IP,
			Created: rec.Created.UTC().Format(time.RFC3339), LastSeen: rec.LastSeen.UTC().Format(time.RFC3339), UserAgent: rec.UserAgent})
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": name, "sessions": out})
}

// handleRevokeUserSession signs one of another person's devices out ({"id": "..."}), or all of them
// ({"all": true}). An admin's own sessions are managed from their Account page, so this refuses their own name.
func (a *App) handleRevokeUserSession(w http.ResponseWriter, r *http.Request) {
	reg := a.Auth.Sessions.Registry
	if reg == nil {
		http.Error(w, "this server does not keep a session list", http.StatusNotFound)
		return
	}
	name := strings.ToLower(r.PathValue("name"))
	if name == actor(r) {
		http.Error(w, "manage your own devices from your Account page", http.StatusBadRequest)
		return
	}
	var body struct {
		ID  string `json:"id"`
		All bool   `json:"all"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	var (
		n   int
		err error
	)
	switch {
	case body.All:
		n, err = reg.RevokeAll(name, "")
	case body.ID != "":
		var ok bool
		if ok, err = reg.Revoke(name, body.ID); err == nil && !ok {
			http.Error(w, "no such session", http.StatusNotFound)
			return
		}
		n = 1
	default:
		http.Error(w, "say which session", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "could not save: "+err.Error(), http.StatusInternalServerError)
		return
	}
	audit.Printf("%s signed out %d session(s) of %q", actor(r), n, name)
	a.recordUser(r, fmt.Sprintf("signed out %d session(s) of %s", n, name))
	writeJSON(w, http.StatusOK, map[string]any{"revoked": n})
}
