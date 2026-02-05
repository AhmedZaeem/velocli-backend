package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/service"
)

func JWTAuth(jwtService *service.JWTService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authz := strings.TrimSpace(c.Get("Authorization"))
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		token := strings.TrimSpace(authz[len("bearer "):])
		sub, err := jwtService.Verify(token)
		if err != nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		c.Locals("subject", sub)
		return c.Next()
	}
}
