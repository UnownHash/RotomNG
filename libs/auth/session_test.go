package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newTestRouter wires a middleware the same way the web server does: session
// endpoints unauthenticated, everything else behind the middleware.
func newTestRouter(mw *Middleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mw.SetupSessionRoutes(router.Group("/api"), discardLogger())

	api := router.Group("/api")
	api.Use(mw.Handler)
	api.GET("/status", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return router
}

func doLogin(t *testing.T, router *gin.Engine, secret string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"secret":"`+secret+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == SessionCookieName {
			return cookie
		}
	}
	t.Fatalf("no %s cookie in response", SessionCookieName)
	return nil
}

// TestSessionCookieAuthenticatesRequests is the regression test for the bug
// this whole feature exists to fix: with a secret configured, the UI could not
// reach any API endpoint.
func TestSessionCookieAuthenticatesRequests(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	loginResponse := doLogin(t, router, "a-secret")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResponse.Code, loginResponse.Body)
	}
	cookie := sessionCookie(t, loginResponse)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	req.Header.Set(SessionRequestHeader, "1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected the session cookie to authenticate the request, got %d", w.Code)
	}
}

func TestSessionCookieHardening(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly so page JavaScript cannot read the token")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("expected path /, got %q", cookie.Path)
	}
	if cookie.Value == "a-secret" {
		t.Error("the cookie must carry a token, never the secret itself")
	}
}

// TestCookieRequiresSessionHeader covers the CSRF defence: the browser attaches the
// cookie to cross-site requests under some conditions, but a cross-site caller
// cannot set a custom header without a preflight.
func TestCookieRequiresSessionHeader(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	// Deliberately no SessionRequestHeader.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for a cookie request without %s, got %d", SessionRequestHeader, w.Code)
	}
}

func TestSessionTTLDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Middleware)
		want time.Duration
	}{
		{"unset falls back to the default", func(*Middleware) {}, DefaultSessionTTL},
		{"explicit ttl is used", func(mw *Middleware) { mw.SetSessionTTL(30 * time.Minute) }, 30 * time.Minute},
		{"zero falls back to the default", func(mw *Middleware) { mw.SetSessionTTL(0) }, DefaultSessionTTL},
		{"negative falls back to the default", func(mw *Middleware) { mw.SetSessionTTL(-5 * time.Minute) }, DefaultSessionTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewMiddleware("a-secret")
			tt.set(mw)
			if got := mw.SessionTTL(); got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestSessionTTLAppliesToCookieAndToken checks the configured lifetime reaches
// both halves of the session: the cookie the browser holds, and the expiry
// signed into the token the server verifies. If they disagreed, a session
// would either die early or outlive its cookie.
func TestSessionTTLAppliesToCookieAndToken(t *testing.T) {
	const ttl = 15 * time.Minute

	mw := NewMiddleware("a-secret")
	mw.SetSessionTTL(ttl)
	router := newTestRouter(mw)

	cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

	if want := int(ttl.Seconds()); cookie.MaxAge != want {
		t.Errorf("expected cookie Max-Age %d, got %d", want, cookie.MaxAge)
	}

	now := time.Now()
	expiry, err := VerifyToken("a-secret", cookie.Value, now)
	if err != nil {
		t.Fatalf("VerifyToken rejected the session token: %v", err)
	}
	// Second-granularity claims plus clock movement between mint and check.
	if delta := expiry.Sub(now.Add(ttl)); delta > 2*time.Second || delta < -2*time.Second {
		t.Errorf("expected the token to expire ~%v out, got %v", ttl, expiry.Sub(now))
	}
}

// TestShortSessionTTLExpires proves a configured TTL actually ends the session
// rather than only being advertised on the cookie.
func TestShortSessionTTLExpires(t *testing.T) {
	mw := NewMiddleware("a-secret")
	mw.SetSessionTTL(time.Second)

	token, err := mw.MintSessionToken(mw.SessionTTL())
	if err != nil {
		t.Fatalf("MintSessionToken returned error: %v", err)
	}

	if _, err := VerifyToken("a-secret", token, time.Now()); err != nil {
		t.Fatalf("token should be valid immediately after minting: %v", err)
	}
	if _, err := VerifyToken("a-secret", token, time.Now().Add(2*time.Second)); !IsErrTokenExpired(err) {
		t.Errorf("expected the token to be expired after its ttl, got %v", err)
	}
}

func TestBearerTokenAuthenticatesRequests(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	token, err := mw.MintSessionToken(time.Hour)
	if err != nil {
		t.Fatalf("MintSessionToken returned error: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid bearer token", "Bearer " + token, http.StatusOK},
		{"garbage bearer token", "Bearer not-a-token", http.StatusUnauthorized},
		{"missing bearer prefix", token, http.StatusUnauthorized},
		{"empty bearer token", "Bearer ", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

// TestSecretRotationInvalidatesSessions documents the only revocation path a
// stateless token has: changing the secret changes the derived signing key.
func TestSecretRotationInvalidatesSessions(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

	mw.SetSecret("rotated-secret")

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	req.Header.Set(SessionRequestHeader, "1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected the pre-rotation session to be rejected, got %d", w.Code)
	}
}

func TestLoginRejectsWrongSecret(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	w := doLogin(t, router, "wrong-secret")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for a wrong secret, got %d", w.Code)
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == SessionCookieName && cookie.Value != "" {
			t.Error("a failed login must not set a session cookie")
		}
	}
}

// TestLoginDisabledWithoutSecret ensures an unauthenticated instance cannot
// have a session minted against an empty secret.
func TestLoginDisabledWithoutSecret(t *testing.T) {
	router := newTestRouter(NewMiddleware(""))

	w := doLogin(t, router, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when no secret is configured, got %d", w.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	mw := NewMiddleware("a-secret")
	router := newTestRouter(mw)

	cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set(SessionRequestHeader, "1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected logout to succeed, got %d", w.Code)
	}
	cleared := sessionCookie(t, w)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("expected logout to clear the cookie, got value=%q maxAge=%d",
			cleared.Value, cleared.MaxAge)
	}
}

func TestAuthMeReportsState(t *testing.T) {
	type meResponse struct {
		AuthRequired  bool `json:"auth_required"`
		Authenticated bool `json:"authenticated"`
	}

	t.Run("no secret configured", func(t *testing.T) {
		router := newTestRouter(NewMiddleware(""))

		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var body meResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body.AuthRequired {
			t.Error("expected auth_required=false when no secret is configured")
		}
		if !body.Authenticated {
			t.Error("expected authenticated=true when no secret is configured")
		}
	})

	t.Run("secret configured, no credential", func(t *testing.T) {
		router := newTestRouter(NewMiddleware("a-secret"))

		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// The probe itself must stay reachable while logged out, otherwise the
		// UI has no way to discover that it needs to log in.
		if w.Code != http.StatusOK {
			t.Fatalf("expected /auth/me to be reachable unauthenticated, got %d", w.Code)
		}
		var body meResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !body.AuthRequired {
			t.Error("expected auth_required=true")
		}
		if body.Authenticated {
			t.Error("expected authenticated=false without a credential")
		}
	})

	t.Run("secret configured, logged in", func(t *testing.T) {
		mw := NewMiddleware("a-secret")
		router := newTestRouter(mw)
		cookie := sessionCookie(t, doLogin(t, router, "a-secret"))

		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.AddCookie(cookie)
		req.Header.Set(SessionRequestHeader, "1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var body meResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !body.AuthRequired || !body.Authenticated {
			t.Errorf("expected auth_required=true and authenticated=true, got %+v", body)
		}
	})
}
