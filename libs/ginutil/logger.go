// Package ginutil provides Gin middleware and engine utilities.
package ginutil

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/logging"
)

// RecoveryMiddleware creates a Gin middleware that recovers from panics,
// logs the panic via slog with a full stack trace, and returns HTTP 500.
func RecoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logging.LogRecovery(logger, "panic recovered in HTTP handler", r)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// LoggerMiddleware creates a Gin middleware that uses the provided logger
// to produce a single structured slog log entry per request using zero-alloc
// LogAttrs calls.
func LoggerMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		status := c.Writer.Status()
		latency := time.Since(start)

		// Determine log level from status code
		var level slog.Level
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		default:
			level = slog.LevelInfo
		}

		// Build attrs slice -- base attrs always present
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("response_body_size", c.Writer.Size()),
		}

		// Add error message if present (single log line, not double)
		if errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String(); errMsg != "" {
			attrs = append(attrs, slog.String("error", errMsg))
		}

		logger.LogAttrs(c.Request.Context(), level, "request completed", attrs...)
	}
}

// NewEngineWithLogger creates a new Gin engine with custom logger middleware
// instead of using gin.Default() which includes the default logger.
//
// Gzip sits inside the logger so the size the logger reports is the size that
// actually went over the wire, not the size before compression.
func NewEngineWithLogger(logger *slog.Logger) *gin.Engine {
	r := gin.New()
	r.Use(RecoveryMiddleware(logger))
	r.Use(LoggerMiddleware(logger))
	r.Use(GzipMiddleware())
	return r
}
