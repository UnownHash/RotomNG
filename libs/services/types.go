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

// RoutesInstaller is an interface for setting up routes on a gin engine.
type RoutesInstaller interface {
	SetupRoutes(r *gin.Engine) error
}

// StatsRegistrar registers stats collection on a gin engine.
type StatsRegistrar interface {
	RegisterGinEngine(r *gin.Engine)
}
