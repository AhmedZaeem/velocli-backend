package handlers

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
	"github.com/velocli/velocli/velocli-backend/internal/service"
)

type AuthHandler struct {
	lemon     *service.LemonSqueezyService
	jwt       *service.JWTService
	customers *repository.CustomersRepository
}

func NewAuthHandler(lemon *service.LemonSqueezyService, jwt *service.JWTService, customers *repository.CustomersRepository) *AuthHandler {
	return &AuthHandler{
		lemon:     lemon,
		jwt:       jwt,
		customers: customers,
	}
}

func (h *AuthHandler) Login() fiber.Handler {
	type request struct {
		LicenseKey string `json:"license_key"`
	}

	return func(c *fiber.Ctx) error {
		var req request
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}
		req.LicenseKey = strings.TrimSpace(req.LicenseKey)
		if req.LicenseKey == "" {
			return fiber.NewError(fiber.StatusBadRequest, "license_key is required")
		}

		validation, err := h.lemon.ValidateLicenseDetailed(c.UserContext(), req.LicenseKey)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid license")
		}
		if !validation.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid license")
		}

		lemonCustomerID := strconv.FormatInt(validation.CustomerID, 10)
		status := normalizeLicenseStatus(validation.Status)
		digest := h.customers.LicenseKeyDigest(req.LicenseKey)
		if err := h.customers.UpsertLicenseKey(c.UserContext(), lemonCustomerID, digest, status, validation.ExpiresAt); err != nil {
			return err
		}

		token, exp, err := h.jwt.Issue(lemonCustomerID)
		if err != nil {
			return err
		}

		return c.JSON(fiber.Map{
			"token":      token,
			"expires_at": exp.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

func normalizeLicenseStatus(v string) domain.SubscriptionStatus {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "active":
		return domain.SubscriptionStatusActive
	case "inactive":
		return domain.SubscriptionStatusActive
	case "expired", "disabled":
		return domain.SubscriptionStatusCancelled
	default:
		return domain.SubscriptionStatusCancelled
	}
}
