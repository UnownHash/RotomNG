package auth

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Log field key constants.
const (
	fieldStatus        = "status"
	fieldError         = "error"
	fieldAuthRequired  = "auth_required"
	fieldAuthenticated = "authenticated"
	statusOK           = "ok"
	statusError        = "error"
)

// loginRequest is the body of a login attempt.
type loginRequest struct {
	Secret string `json:"secret"`
}

// SetupSessionRoutes registers the unauthenticated session endpoints on group.
//
// These deliberately sit outside the authenticated route group: they are how a
// browser obtains a credential in the first place, so requiring one to reach
// them would lock the UI out permanently.
func (mw *Middleware) SetupSessionRoutes(group *gin.RouterGroup, logger *slog.Logger) {
	group.GET("/auth/me", mw.handleMe)
	group.POST("/auth/login", func(c *gin.Context) { mw.handleLogin(c, logger) })
	group.POST("/auth/logout", mw.handleLogout)
}

// handleMe reports whether authentication is required, and whether this
// request already carries a valid credential. The UI polls this on load to
// decide between rendering the app and rendering the login form.
func (mw *Middleware) handleMe(c *gin.Context) {
	secret := mw.currentSecret()
	if secret == "" {
		c.JSON(http.StatusOK, gin.H{
			fieldStatus:        statusOK,
			fieldAuthRequired:  false,
			fieldAuthenticated: true,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		fieldStatus:        statusOK,
		fieldAuthRequired:  true,
		fieldAuthenticated: mw.authenticate(c, secret),
	})
}

// handleLogin exchanges the configured secret for a session cookie.
func (mw *Middleware) handleLogin(c *gin.Context, logger *slog.Logger) {
	if !mw.Enabled() {
		c.JSON(http.StatusBadRequest, gin.H{
			fieldStatus: statusError,
			fieldError:  "authentication is not enabled",
		})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			fieldStatus: statusError,
			fieldError:  "invalid request body",
		})
		return
	}

	if !mw.CheckSecret(req.Secret) {
		// Logged so operators can spot brute-force attempts against an
		// endpoint that is, by necessity, unauthenticated.
		logger.LogAttrs(c.Request.Context(), slog.LevelWarn, "failed UI login attempt",
			slog.String("remote_addr", c.ClientIP()))
		c.JSON(http.StatusUnauthorized, gin.H{
			fieldStatus: statusError,
			fieldError:  "invalid secret",
		})
		return
	}

	ttl := mw.SessionTTL()
	token, err := mw.MintSessionToken(ttl)
	if err != nil {
		logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to mint session token",
			slog.String(fieldError, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			fieldStatus: statusError,
			fieldError:  "failed to create session",
		})
		return
	}

	// The cookie's Max-Age mirrors the token's exp so the browser drops it at
	// the same moment the server stops honouring it.
	setSessionCookie(c, token, int(ttl.Seconds()))
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "UI login succeeded",
		slog.String("remote_addr", c.ClientIP()),
		slog.Duration("session_ttl", ttl))
	c.JSON(http.StatusOK, gin.H{
		fieldStatus:        statusOK,
		fieldAuthRequired:  true,
		fieldAuthenticated: true,
	})
}

// handleLogout clears the session cookie.
func (mw *Middleware) handleLogout(c *gin.Context) {
	setSessionCookie(c, "", -1)
	c.JSON(http.StatusOK, gin.H{
		fieldStatus:        statusOK,
		fieldAuthRequired:  mw.Enabled(),
		fieldAuthenticated: false,
	})
}

// setSessionCookie writes the session cookie with the hardening flags the
// token's security depends on: HttpOnly so JavaScript cannot read it, and
// SameSite=Strict so a cross-site navigation never carries it.
func setSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteStrictMode)
	// Secure would make the cookie undeliverable over plain HTTP, which is a
	// supported way to run this, so it is set only when the connection really
	// is TLS -- either directly or via a proxy that says so.
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetCookie(SessionCookieName, token, maxAge, "/", "", secure, true)
}
