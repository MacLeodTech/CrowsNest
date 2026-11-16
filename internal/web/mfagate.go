package web

import (
	"net/http"
	"strings"

	"github.com/macleodtech/crowsnest/internal/auth"
)

// mfaExempt are the paths an admin may use before setting up an authenticator app: their Account page and what it
// calls, sign-out, and the static files the page needs.
func mfaExempt(path string) bool {
	return path == "/account" || path == "/logout" || path == "/healthz" || strings.HasPrefix(path, "/api/account/") ||
		strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/login")
}

// MFAGate wraps next so that, while Settings requires an authenticator app for their role, a user who signs
// in with a password and has no authenticator yet can reach only their Account page until they set one up. Pages
// redirect there; API calls get a 403 that says why. Admins who sign in through Google or another provider are not
// asked: their provider does the second factor.
func (a *App) MFAGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.mfaEnrolmentNeeded(r) && !mfaExempt(r.URL.Path) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "set up an authenticator app on your Account page first: this server requires one for your account", http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/account#totp", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) mfaEnrolmentNeeded(r *http.Request) bool {
	if a.Settings == nil || a.MFA == nil {
		return false
	}
	session, err := auth.DecodeSessionCookie(a.Auth.Sessions, r)
	if err != nil {
		return false // not signed in: the normal sign-in flow applies
	}
	return a.mfaRequiredFor(session.Username)
}

// mfaRequiredFor reports whether user must set up an authenticator app (or a passkey) before doing anything else:
// the setting for their role is on, they can sign in with a password, and they have neither. Players who are
// linked to Google or another provider are not asked, because the provider does the second factor.
func (a *App) mfaRequiredFor(user string) bool {
	if a.Settings == nil || a.MFA == nil {
		return false
	}
	v := a.Settings.Get()
	admin := a.Auth.Sessions.RoleFor(user) == auth.RoleAdmin
	if (admin && !v.RequireAdminMFA) || (!admin && !v.RequirePlayerMFA) {
		return false
	}
	if !a.Auth.HasPassword(user) || a.MFA.Enabled(user) || (a.Auth.Passkeys != nil && a.Auth.Passkeys.Store.Count(user) > 0) {
		return false
	}
	return admin || !a.hasOAuth(user)
}

// hasOAuth reports whether user can sign in through Google or another OpenID Connect provider.
func (a *App) hasOAuth(user string) bool {
	if a.Auth.Store != nil {
		if rec, ok := a.Auth.Store.Lookup(user); ok && (rec.GoogleEmail != "" || rec.OIDCEmail != "") {
			return true
		}
	}
	return a.Google != nil && a.Google.Allows(user)
}
