package admin

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const csrfCookieName = "ztna_csrf"
const csrfHeaderName = "X-CSRF-Token"

// csrfProtect is middleware that enforces double-submit cookie CSRF protection
// on state-changing requests (POST, PUT, PATCH, DELETE).
// GET, HEAD, OPTIONS are exempt.
// Requests using Bearer token auth (not cookies) are also exempt since
// they're not vulnerable to CSRF.
func csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe methods are exempt
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			ensureCSRFCookie(w, r)
			next.ServeHTTP(w, r)
			return
		}

		// Bearer token auth is not vulnerable to CSRF (browser doesn't auto-send it)
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		// For cookie-based auth, validate CSRF token
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, "missing CSRF token", http.StatusForbidden)
			return
		}

		headerToken := r.Header.Get(csrfHeaderName)
		if headerToken == "" {
			http.Error(w, "missing X-CSRF-Token header", http.StatusForbidden)
			return
		}

		if cookie.Value != headerToken {
			http.Error(w, "CSRF token mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ensureCSRFCookie sets the CSRF cookie if not already present.
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(csrfCookieName); err == nil {
		return // already has cookie
	}
	token := generateCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: false, // must be readable by JavaScript for double-submit
		SameSite: http.SameSiteStrictMode,
	})
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
