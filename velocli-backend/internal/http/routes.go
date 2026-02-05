package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
	"github.com/velocli/velocli/velocli-backend/internal/http/middleware"
	"strings"
)

func registerRoutes(app *fiber.App, cfg config.Config, deps Deps) {
	app.Get("/healthz", handlers.Healthz())

	apiV1 := app.Group("/api/v1")
	if deps.Auth != nil {
		app.Post("/auth/login", deps.Auth.Login())
	}
	if deps.LemonWebhook != nil {
		app.Post("/webhooks/lemon", deps.LemonWebhook.Handle())
		apiV1.Post("/webhooks/lemon", deps.LemonWebhook.Handle())
	}
	if !strings.EqualFold(cfg.Env, "prod") && deps.TestSimulator != nil {
		apiV1.Post("/test/simulate-purchase", deps.TestSimulator.SimulatePurchase())
	}
	if deps.Bricks != nil && deps.JWT != nil {
		protected := app.Group("", middleware.JWTAuth(deps.JWT))
		protected.Get("/bricks/:brick_id", deps.Bricks.GetBrick())
	}
}
