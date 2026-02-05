package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
)

func registerRoutes(app *fiber.App, cfg config.Config) {
	app.Get("/healthz", handlers.Healthz())
}
