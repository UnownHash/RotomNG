// Package auth provides authentication middleware for HTTP handlers.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const bearerPrefix = "Bearer "

// Middleware validates requests using a shared secret header.
type Middleware struct {
	secret atomic.Pointer[string]
	// sessionTTL is stored as an int64 nanosecond count so config reload can
	// change it without racing an in-flight login. Zero means DefaultSessionTTL.
	sessionTTLNanos atomic.Int64
}

// NewMiddleware creates a gin middleware that checks for authentication.
//
// A request is accepted when it carries any one of:
//   - the X-Rotom-Secret header matching the configured secret (machine clients)
//   - an Authorization: Bearer header holding a valid session token
//   - the session cookie holding a valid token, plus the X-Rotom-Session header
//
// Returns 401 Unauthorized otherwise. When no secret is configured, every
// request is allowed through.
func NewMiddleware(expectedSecret string) *Middleware {
	mw := &Middleware{}
	mw.secret.Store(&expectedSecret)
	return mw
}

// Handler is a gin middleware that authenticates the request.
func (mw *Middleware) Handler(ginContext *gin.Context) {
	if !mw.Allow(ginContext) {
		ginContext.Status(http.StatusUnauthorized)
		ginContext.Abort()
		return
	}
	ginContext.Next()
}

// Allow reports whether the request carries an acceptable credential, without
// touching the response or the handler chain. Handlers reached outside the
// authenticated route group -- gin's NoRoute, for one -- use this to make the
// same decision Handler would.
//
// Returns true when no secret is configured, matching Handler's behaviour of
// letting every request through on an unauthenticated instance.
func (mw *Middleware) Allow(ginContext *gin.Context) bool {
	secret := mw.currentSecret()
	if secret == "" {
		return true
	}
	return mw.authenticate(ginContext, secret)
}

// SetSessionTTL sets how long newly minted UI sessions stay valid. Values <= 0
// select DefaultSessionTTL.
//
// Changing this affects only sessions minted afterwards: a token's expiry is
// baked into its signed claims, so shortening the TTL does not retroactively
// cut short sessions already issued. Rotating the secret is what ends those.
func (mw *Middleware) SetSessionTTL(ttl time.Duration) {
	if ttl < 0 {
		ttl = 0
	}
	mw.sessionTTLNanos.Store(int64(ttl))
}

// SessionTTL returns the lifetime applied to newly minted sessions.
func (mw *Middleware) SessionTTL() time.Duration {
	if ttl := time.Duration(mw.sessionTTLNanos.Load()); ttl > 0 {
		return ttl
	}
	return DefaultSessionTTL
}

// SetSecret updates the expected secret value atomically. Rotating the secret
// also invalidates every outstanding session token, since the token signing
// key is derived from it.
func (mw *Middleware) SetSecret(secret string) {
	mw.secret.Store(&secret)
}

// Enabled reports whether a secret is configured, and therefore whether
// clients need to authenticate at all.
func (mw *Middleware) Enabled() bool {
	return mw.currentSecret() != ""
}

// CheckSecret reports whether provided matches the configured secret. It
// returns false when no secret is configured, so a login attempt cannot mint a
// token on an unauthenticated instance.
func (mw *Middleware) CheckSecret(provided string) bool {
	secret := mw.currentSecret()
	if secret == "" {
		return false
	}
	return secretsEqual(provided, secret)
}

// MintSessionToken issues a session token bound to the current secret.
func (mw *Middleware) MintSessionToken(ttl time.Duration) (string, error) {
	return MintToken(mw.currentSecret(), time.Now(), ttl)
}

// VerifySessionToken checks a token against the current secret.
func (mw *Middleware) VerifySessionToken(token string) error {
	_, err := VerifyToken(mw.currentSecret(), token, time.Now())
	return err
}

// authenticate reports whether the request carries any acceptable credential.
func (mw *Middleware) authenticate(ginContext *gin.Context, secret string) bool {
	if secretsEqual(ginContext.GetHeader(SecretRequestHeader), secret) {
		return true
	}

	authorization := ginContext.GetHeader("Authorization")
	if token, found := strings.CutPrefix(authorization, bearerPrefix); found {
		if _, err := VerifyToken(secret, token, time.Now()); err == nil {
			return true
		}
	}

	// Cookie credentials are only honoured on requests carrying the session
	// header. The browser attaches the cookie automatically, so without this
	// check a cross-site form post would ride an operator's live session; a
	// custom header cannot be set cross-origin without a preflight we never
	// answer.
	if ginContext.GetHeader(SessionRequestHeader) == "" {
		return false
	}
	cookie, err := ginContext.Cookie(SessionCookieName)
	if err != nil || cookie == "" {
		return false
	}
	_, err = VerifyToken(secret, cookie, time.Now())
	return err == nil
}

func (mw *Middleware) currentSecret() string {
	secret := mw.secret.Load()
	if secret == nil {
		return ""
	}
	return *secret
}

func secretsEqual(provided, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
