// Package auth provides authentication middleware for HTTP handlers.
package auth

import (
	"crypto/subtle"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// Middleware validates requests using a shared secret header.
type Middleware struct {
	secret atomic.Pointer[string]
}

// NewMiddleware creates a gin middleware that checks for authentication
// via the X-Rotom-Secret header.
// Returns 401 Unauthorized if the header doesn't match the expected secret value.
func NewMiddleware(expectedSecret string) *Middleware {
	mw := &Middleware{}
	mw.secret.Store(&expectedSecret)
	return mw
}

// Handler is a gin middleware that checks the X-Rotom-Secret header.
func (mw *Middleware) Handler(ginContext *gin.Context) {
	expectedSecret := mw.secret.Load()
	providedSecret := ginContext.GetHeader("X-Rotom-Secret")
	if expectedSecret != nil && *expectedSecret != "" && subtle.ConstantTimeCompare([]byte(providedSecret), []byte(*expectedSecret)) != 1 {
		ginContext.Status(http.StatusUnauthorized)
		ginContext.Abort()
		return
	}
	ginContext.Next()
}

// SetSecret updates the expected secret value atomically.
func (mw *Middleware) SetSecret(secret string) {
	mw.secret.Store(&secret)
}
