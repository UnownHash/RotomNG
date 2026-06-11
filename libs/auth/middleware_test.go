package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		expectedSecret string
		xRotomHeader   string
		expectedStatus int
		description    string
	}{
		{
			name:           "Valid X-Rotom-Secret header",
			expectedSecret: "test-secret",
			xRotomHeader:   "test-secret",
			expectedStatus: http.StatusOK,
			description:    "Should accept valid X-Rotom-Secret header",
		},
		{
			name:           "Invalid X-Rotom-Secret header",
			expectedSecret: "test-secret",
			xRotomHeader:   "wrong-secret",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject invalid X-Rotom-Secret header",
		},
		{
			name:           "No authentication headers",
			expectedSecret: "test-secret",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject request with no authentication headers",
		},
		{
			name:           "Empty X-Rotom-Secret",
			expectedSecret: "test-secret",
			xRotomHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject empty X-Rotom-Secret header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new Gin router for each test
			router := gin.New()

			// Apply the auth middleware
			authMiddleware := NewMiddleware(tt.expectedSecret)
			router.Use(authMiddleware.Handler)

			// Add a test endpoint
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			// Create a test request
			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Set headers based on test case
			if tt.xRotomHeader != "" {
				req.Header.Set("X-Rotom-Secret", tt.xRotomHeader)
			}

			// Create a response recorder
			w := httptest.NewRecorder()

			// Perform the request
			router.ServeHTTP(w, req)

			// Assert the expected status code
			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}

			// Additional assertions based on expected status
			if tt.expectedStatus == http.StatusOK {
				if !strings.Contains(w.Body.String(), "success") {
					t.Errorf("%s: expected response to contain 'success', got %s", tt.description, w.Body.String())
				}
			}
			// For unauthorized requests, we only check the status code (no response body expected)
		})
	}
}

func TestAuthMiddleware_EmptySecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test that middleware still works with empty expected secret
	router := gin.New()
	authMiddleware := NewMiddleware("")
	router.Use(authMiddleware.Handler)

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Test with no headers - should pass since expected secret is empty
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Errorf("Expected response to contain 'success', got %s", w.Body.String())
	}
}
