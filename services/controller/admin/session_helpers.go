package admin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	sessionEmailKey     contextKey = "session_email"
	sessionUserIDKey    contextKey = "session_uid"
	sessionWorkspaceKey contextKey = "session_wid"
	sessionWSlugKey     contextKey = "session_wslug"
	sessionWRoleKey     contextKey = "session_wrole"
)

const sessionCookieName = "ztna_session"
const jwtIssuer = "ztna-controller"

// signSessionJWT creates a signed JWT containing the user's email.
func (s *Server) signSessionJWT(email string) (string, error) {
	if len(s.JWTSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	claims := jwt.MapClaims{
		"sub": email,
		"iss": jwtIssuer,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.JWTSecret)
}

// verifySessionJWT validates a JWT and returns the email claim.
func (s *Server) verifySessionJWT(tokenStr string) (string, error) {
	if len(s.JWTSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.JWTSecret, nil
	})
	if err != nil || !tok.Valid {
		return "", fmt.Errorf("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}
	email, ok := claims["sub"].(string)
	if !ok || email == "" {
		return "", fmt.Errorf("missing subject")
	}
	return email, nil
}

// setSessionCookie writes the JWT as an http-only session cookie.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   s.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// sessionFromRequest extracts and verifies the JWT from the session cookie or
// the Authorization: Bearer header.
func (s *Server) sessionFromRequest(r *http.Request) (string, error) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return s.verifySessionJWT(cookie.Value)
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return s.verifySessionJWT(strings.TrimPrefix(auth, "Bearer "))
	}
	return "", fmt.Errorf("no session")
}

// withSessionEmail returns a new context carrying the authenticated email.
func withSessionEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, sessionEmailKey, email)
}

// sessionEmailFromContext retrieves the email stored by withSessionEmail.
func sessionEmailFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionEmailKey).(string)
	return v
}

// signWorkspaceJWT creates a JWT with workspace claims.
func (s *Server) signWorkspaceJWT(email, userID, wsID, wsSlug, wsRole string) (string, error) {
	if len(s.JWTSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	claims := jwt.MapClaims{
		"sub":   email,
		"uid":   userID,
		"wid":   wsID,
		"wslug": wsSlug,
		"wrole": wsRole,
		"iss":   jwtIssuer,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.JWTSecret)
}

// workspaceClaimsFromJWT validates a JWT and extracts workspace claims.
func workspaceClaimsFromJWT(tokenStr string, secret []byte) (email, userID, wsID, wsSlug, wsRole string, err error) {
	if len(secret) == 0 {
		return "", "", "", "", "", fmt.Errorf("JWT_SECRET not configured")
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return "", "", "", "", "", fmt.Errorf("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", "", "", "", fmt.Errorf("invalid claims")
	}
	if iss, _ := claims["iss"].(string); iss != "" && iss != jwtIssuer {
		return "", "", "", "", "", fmt.Errorf("invalid issuer")
	}
	email, _ = claims["sub"].(string)
	userID, _ = claims["uid"].(string)
	wsID, _ = claims["wid"].(string)
	wsSlug, _ = claims["wslug"].(string)
	wsRole, _ = claims["wrole"].(string)
	if email == "" {
		return "", "", "", "", "", fmt.Errorf("missing subject")
	}
	return email, userID, wsID, wsSlug, wsRole, nil
}

func withWorkspace(ctx context.Context, userID, wsID, wsSlug, wsRole string) context.Context {
	ctx = context.WithValue(ctx, sessionUserIDKey, userID)
	ctx = context.WithValue(ctx, sessionWorkspaceKey, wsID)
	ctx = context.WithValue(ctx, sessionWSlugKey, wsSlug)
	ctx = context.WithValue(ctx, sessionWRoleKey, wsRole)
	return ctx
}

func userIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionUserIDKey).(string)
	return v
}

func workspaceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionWorkspaceKey).(string)
	return v
}

func workspaceSlugFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionWSlugKey).(string)
	return v
}

func workspaceRoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionWRoleKey).(string)
	return v
}

// allClaims holds all relevant JWT claim fields.
type allClaims struct {
	email    string
	userID   string
	wsID     string
	wsSlug   string
	wsRole   string
	aud      string
	jti      string
	deviceID string
}

// parseAllClaims parses a JWT and returns all relevant claims.
func parseAllClaims(tokenStr string, secret []byte) (allClaims, error) {
	var c allClaims
	if tokenStr == "" || len(secret) == 0 {
		return c, fmt.Errorf("no token")
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return c, fmt.Errorf("invalid token")
	}
	m, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return c, fmt.Errorf("invalid claims")
	}
	if iss, _ := m["iss"].(string); iss != "" && iss != jwtIssuer {
		return c, fmt.Errorf("invalid issuer")
	}
	c.email, _ = m["sub"].(string)
	c.userID, _ = m["uid"].(string)
	c.wsID, _ = m["wid"].(string)
	c.wsSlug, _ = m["wslug"].(string)
	c.wsRole, _ = m["wrole"].(string)
	c.jti, _ = m["jti"].(string)
	c.deviceID, _ = m["did"].(string)
	if aud, ok := m["aud"].(string); ok {
		c.aud = aud
	}
	if c.email == "" {
		return c, fmt.Errorf("missing subject")
	}
	return c, nil
}

// getTokenFromRequest extracts the raw JWT string from the session cookie or Authorization header.
func (s *Server) getTokenFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	return ""
}

// signAdminJWT creates an admin JWT with aud:"admin" and jti:sessionID.
func (s *Server) signAdminJWT(email, userID, wsID, wsSlug, wsRole, sessionID string) (string, error) {
	if len(s.JWTSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	claims := jwt.MapClaims{
		"sub":   email,
		"uid":   userID,
		"wid":   wsID,
		"wslug": wsSlug,
		"wrole": wsRole,
		"aud":   "admin",
		"jti":   sessionID,
		"iss":   jwtIssuer,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.JWTSecret)
}

// signDeviceJWT creates a device JWT with aud:"device", 15-minute expiry, and jti:sessionID.
func (s *Server) signDeviceJWT(email, userID, wsID, wsSlug, wsRole, deviceID, sessionID string) (string, error) {
	if len(s.JWTSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	claims := jwt.MapClaims{
		"sub":   email,
		"uid":   userID,
		"wid":   wsID,
		"wslug": wsSlug,
		"wrole": wsRole,
		"aud":   "device",
		"did":   deviceID,
		"jti":   sessionID,
		"iss":   jwtIssuer,
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.JWTSecret)
}
