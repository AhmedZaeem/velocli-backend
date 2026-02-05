package http

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
	"github.com/velocli/velocli/velocli-backend/internal/service"
)

type Deps struct {
	LemonWebhook *handlers.LemonWebhookHandler
	Auth         *handlers.AuthHandler
	Bricks       *handlers.BricksHandler
	JWT          *service.JWTService
}

func NewApp(cfg config.Config, deps Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			status := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				status = e.Code
			}
			return c.Status(status).JSON(fiber.Map{
				"error": http.StatusText(status),
			})
		},
	})

	registerRoutes(app, cfg, deps)
	return app
}
