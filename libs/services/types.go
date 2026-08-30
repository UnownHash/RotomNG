package services

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware is an interface for authentication middleware.
type AuthMiddleware interface {
	Handler(ginContext *gin.Context)
}

// SessionAuthMiddleware is an AuthMiddleware that can also issue browser
// session credentials. Middleware implementing it gets its session endpoints
// registered on the unauthenticated side of the API group.
type SessionAuthMiddleware interface {
	AuthMiddleware
	SetupSessionRoutes(group *gin.RouterGroup, logger *slog.Logger)
}

// RequestAuthorizer is an AuthMiddleware that can decide a request without
// running the rest of the handler chain. Handlers that sit outside the
// authenticated route group -- gin's NoRoute -- use it to apply the same
// credential check the group would have applied.
type RequestAuthorizer interface {
	AuthMiddleware
	Allow(ginContext *gin.Context) bool
}

// RoutesInstaller is an interface for setting up routes on a gin engine.
type RoutesInstaller interface {
	SetupRoutes(r *gin.Engine) error
}

// StatsRegistrar registers stats collection on a gin engine.
type StatsRegistrar interface {
	RegisterGinEngine(r *gin.Engine)
}
