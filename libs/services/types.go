package services

import "github.com/gin-gonic/gin"

// AuthMiddleware is an interface for authentication middleware.
type AuthMiddleware interface {
	Handler(ginContext *gin.Context)
}

// RoutesInstaller is an interface for setting up routes on a gin engine.
type RoutesInstaller interface {
	SetupRoutes(r *gin.Engine) error
}

// StatsRegistrar registers stats collection on a gin engine.
type StatsRegistrar interface {
	RegisterGinEngine(r *gin.Engine)
}
