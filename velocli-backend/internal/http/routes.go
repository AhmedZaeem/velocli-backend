package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
)

func registerRoutes(app *fiber.App, cfg config.Config, deps Deps) {
	app.Get("/healthz", handlers.Healthz())
	if deps.Auth != nil {
		app.Post("/auth/login", deps.Auth.Login())
	}
	if deps.LemonWebhook != nil {
		app.Post("/webhooks/lemon", deps.LemonWebhook.Handle())
	}
}
