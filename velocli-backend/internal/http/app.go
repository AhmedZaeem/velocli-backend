package http

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
)

type Deps struct {
	LemonWebhook *handlers.LemonWebhookHandler
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
