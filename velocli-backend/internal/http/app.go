package http

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/config"
)

func NewApp(cfg config.Config) *fiber.App {
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

	registerRoutes(app, cfg)
	return app
}
