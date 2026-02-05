package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
)

type BricksHandler struct {
	customers *repository.CustomersRepository
}

func NewBricksHandler(customers *repository.CustomersRepository) *BricksHandler {
	return &BricksHandler{customers: customers}
}

func (h *BricksHandler) GetBrick() fiber.Handler {
	return func(c *fiber.Ctx) error {
		subject, _ := c.Locals("subject").(string)
		if subject == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		customer, err := h.customers.GetByLemonCustomerID(c.UserContext(), subject)
		if err != nil {
			return err
		}
		if customer == nil {
			return c.SendStatus(fiber.StatusForbidden)
		}
		if customer.SubscriptionStatus != domain.SubscriptionStatusActive {
			return c.SendStatus(fiber.StatusForbidden)
		}
		if customer.ExpiresAt != nil && customer.ExpiresAt.Before(time.Now().UTC()) {
			return c.SendStatus(fiber.StatusForbidden)
		}

		return c.SendStatus(fiber.StatusNotFound)
	}
}
