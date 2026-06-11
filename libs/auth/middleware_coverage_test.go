package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mw := NewMiddleware("initial-secret")

	router := gin.New()
	router.Use(mw.Handler)
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Verify initial secret works
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Rotom-Secret", "initial-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with initial secret, got %d", w.Code)
	}

	// Update secret
	mw.SetSecret("new-secret")

	// Old secret should now fail
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Rotom-Secret", "initial-secret")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with old secret after SetSecret, got %d", w.Code)
	}

	// New secret should work
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Rotom-Secret", "new-secret")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with new secret, got %d", w.Code)
	}
}
