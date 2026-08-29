package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SessionCookieName is the cookie the UI stores its session token in. The
// cookie is set HttpOnly so browser JavaScript can never read the token, which
// keeps an XSS bug in the UI from turning into a stolen credential.
const SessionCookieName = "rotom_session"

// SessionRequestHeader must be present on any request authenticated by cookie.
// A cross-site form or image tag cannot set a custom header without triggering
// a CORS preflight the server never answers, so requiring it blocks CSRF even
// if a browser ignores SameSite.
const SessionRequestHeader = "X-Rotom-Session"

// DefaultSessionTTL is how long a UI session stays valid before the operator
// has to log in again. Callers can override it per-middleware with
// SetSessionTTL; this is the fallback when they do not.
const DefaultSessionTTL = 24 * time.Hour

// signingKeyLabel domain-separates the token signing key from the raw config
// secret, so a token signature can never be confused with, or used to probe,
// the secret itself.
const signingKeyLabel = "rotom-ng ui session v1"

// tokenSubject identifies the single-operator session these tokens represent.
// It exists so a future multi-user scheme can tell old tokens apart.
const tokenSubject = "rotom-ui"

var (
	errTokenInvalid = errors.New("session token is invalid")
	errTokenExpired = errors.New("session token has expired")
)

// NewErrTokenInvalid returns the sentinel error for a malformed or badly
// signed token.
func NewErrTokenInvalid() error { return errTokenInvalid }

// IsErrTokenInvalid reports whether err is the invalid-token sentinel.
func IsErrTokenInvalid(err error) bool { return errors.Is(err, errTokenInvalid) }

// NewErrTokenExpired returns the sentinel error for a well-formed token whose
// expiry has passed.
func NewErrTokenExpired() error { return errTokenExpired }

// IsErrTokenExpired reports whether err is the expired-token sentinel.
func IsErrTokenExpired(err error) bool { return errors.Is(err, errTokenExpired) }

// tokenHeader is the JWT header. Only HS256 is ever minted, and VerifyToken
// requires exactly this value rather than dispatching on whatever the token
// claims -- that check is what makes algorithm-confusion attacks (alg: none,
// or an RS256 public key replayed as an HMAC secret) impossible here.
type tokenHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// tokenClaims is the JWT payload.
type tokenClaims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

// signingKey derives the HMAC key for tokens from the configured secret.
//
// Deriving rather than generating a random key at startup gives two properties
// worth having: sessions survive a restart, and rotating the secret (including
// via config reload) invalidates every outstanding token for free. The latter
// is the only revocation mechanism a stateless token has.
func signingKey(secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingKeyLabel))
	return mac.Sum(nil)
}

func sign(key []byte, signingInput string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingInput))
	return b64.EncodeToString(mac.Sum(nil))
}

// MintToken creates a signed HS256 token valid for ttl, bound to secret.
func MintToken(secret string, now time.Time, ttl time.Duration) (string, error) {
	headerJSON, err := json.Marshal(tokenHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("failed to encode token header: %w", err)
	}
	claimsJSON, err := json.Marshal(tokenClaims{
		Sub: tokenSubject,
		Iat: now.Unix(),
		Exp: now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode token claims: %w", err)
	}

	signingInput := b64.EncodeToString(headerJSON) + "." + b64.EncodeToString(claimsJSON)
	return signingInput + "." + sign(signingKey(secret), signingInput), nil
}

// VerifyToken checks a token's signature and expiry against secret. It returns
// the expiry time on success.
func VerifyToken(secret, token string, now time.Time) (time.Time, error) {
	headerB64, rest, found := strings.Cut(token, ".")
	if !found {
		return time.Time{}, errTokenInvalid
	}
	claimsB64, signatureB64, found := strings.Cut(rest, ".")
	if !found {
		return time.Time{}, errTokenInvalid
	}

	// Compare the signature before decoding any claims, so malformed or
	// hostile payloads are never parsed on an unauthenticated path.
	expected := sign(signingKey(secret), headerB64+"."+claimsB64)
	if !hmac.Equal([]byte(signatureB64), []byte(expected)) {
		return time.Time{}, errTokenInvalid
	}

	headerJSON, err := b64.DecodeString(headerB64)
	if err != nil {
		return time.Time{}, errTokenInvalid
	}
	var header tokenHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return time.Time{}, errTokenInvalid
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		return time.Time{}, errTokenInvalid
	}

	claimsJSON, err := b64.DecodeString(claimsB64)
	if err != nil {
		return time.Time{}, errTokenInvalid
	}
	var claims tokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return time.Time{}, errTokenInvalid
	}
	if claims.Sub != tokenSubject {
		return time.Time{}, errTokenInvalid
	}

	expiry := time.Unix(claims.Exp, 0)
	if !now.Before(expiry) {
		return time.Time{}, errTokenExpired
	}
	return expiry, nil
}
