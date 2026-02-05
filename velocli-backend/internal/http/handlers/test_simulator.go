package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
)

type TestSimulatorHandler struct {
	customers *repository.CustomersRepository
}

func NewTestSimulatorHandler(customers *repository.CustomersRepository) *TestSimulatorHandler {
	return &TestSimulatorHandler{customers: customers}
}

func (h *TestSimulatorHandler) SimulatePurchase() fiber.Handler {
	return func(c *fiber.Ctx) error {
		const (
			email      = "test@example.com"
			licenseKey = "VELO-TEST-KEY-123"
		)

		digest := h.customers.LicenseKeyDigest(licenseKey)
		expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)

		if err := h.customers.UpsertLicenseKey(c.UserContext(), email, digest, domain.SubscriptionStatusActive, &expiresAt); err != nil {
			return err
		}

		return c.JSON(fiber.Map{
			"lemon_customer_id": email,
			"license_key":       licenseKey,
			"status":            "active",
			"expires_at":        expiresAt.Format(time.RFC3339),
		})
	}
}

