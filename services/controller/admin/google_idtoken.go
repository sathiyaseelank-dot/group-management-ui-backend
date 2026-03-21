package admin

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const jwksTTL = time.Hour
const googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// jwksCache is an in-memory cache of RSA public keys fetched from Google's JWKS endpoint.
type jwksCache struct {
	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

var globalJwksCache = &jwksCache{
	keys: make(map[string]*rsa.PublicKey),
}

func (c *jwksCache) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.fetchedAt) < jwksTTL {
		if key, ok := c.keys[kid]; ok {
			return key, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleJWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jwks: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks: fetch: %w", err)
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Alg string `json:"alg"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("jwks: decode: %w", err)
	}

	c.keys = make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Alg != "RS256" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := new(big.Int).SetBytes(eBytes)
		n := new(big.Int).SetBytes(nBytes)
		c.keys[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}
	c.fetchedAt = time.Now()

	key, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("jwks: key not found for kid %q", kid)
	}
	return key, nil
}

// googleIDClaims holds the claims extracted from a Google ID token.
type googleIDClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Exp           int64  `json:"exp"`
	Nonce         string `json:"nonce,omitempty"`
}

// validateGoogleIDToken validates a Google-issued ID token using JWKS.
// No external dependencies — uses stdlib crypto/rsa, math/big, encoding/base64.
func validateGoogleIDToken(ctx context.Context, rawToken, expectedClientID string) (*googleIDClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	pubKey, err := globalJwksCache.getKey(ctx, header.Kid)
	if err != nil {
		return nil, err
	}

	// Verify RS256 signature
	signingInput := parts[0] + "." + parts[1]
	h := sha256.Sum256([]byte(signingInput))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, h[:], sig); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims googleIDClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	validIssuers := map[string]bool{
		"https://accounts.google.com": true,
		"accounts.google.com":         true,
	}
	if !validIssuers[claims.Iss] {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Iss)
	}
	if claims.Aud != expectedClientID {
		return nil, fmt.Errorf("invalid audience: %s", claims.Aud)
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}
	if !claims.EmailVerified {
		return nil, fmt.Errorf("email not verified")
	}
	if claims.Sub == "" {
		return nil, fmt.Errorf("missing sub claim")
	}

	return &claims, nil
}
