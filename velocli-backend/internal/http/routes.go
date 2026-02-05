package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
	"github.com/velocli/velocli/velocli-backend/internal/http/middleware"
)

func registerRoutes(app *fiber.App, cfg config.Config, deps Deps) {
	app.Get("/healthz", handlers.Healthz())
	if deps.Auth != nil {
		app.Post("/auth/login", deps.Auth.Login())
	}
	if deps.LemonWebhook != nil {
		app.Post("/webhooks/lemon", deps.LemonWebhook.Handle())
	}
	if deps.Bricks != nil && deps.JWT != nil {
		protected := app.Group("", middleware.JWTAuth(deps.JWT))
		protected.Get("/bricks/:brick_id", deps.Bricks.GetBrick())
	}
}
