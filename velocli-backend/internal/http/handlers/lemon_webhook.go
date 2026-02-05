package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
	"github.com/velocli/velocli/velocli-backend/internal/integrations/lemonsqueezy"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
)

type LemonWebhookHandler struct {
	webhookSecret []byte
	storeID       int64
	verifySignature bool
	customers     *repository.CustomersRepository
}

func NewLemonWebhookHandler(webhookSecret string, storeID int64, verifySignature bool, customers *repository.CustomersRepository) *LemonWebhookHandler {
	return &LemonWebhookHandler{
		webhookSecret: []byte(webhookSecret),
		storeID:       storeID,
		verifySignature: verifySignature,
		customers:     customers,
	}
}

func (h *LemonWebhookHandler) Handle() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		if ctx == nil {
			ctx = context.Background()
		}

		rawBody := c.Body()
		if h.verifySignature {
			if !lemonsqueezy.VerifySignature(rawBody, h.webhookSecret, c.Get("X-Signature")) {
				return c.SendStatus(fiber.StatusUnauthorized)
			}
		}

		var env lemonsqueezy.WebhookEnvelope
		if err := json.Unmarshal(rawBody, &env); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}

		eventName := strings.TrimSpace(env.Meta.EventName)
		if headerEvent := strings.TrimSpace(c.Get("X-Event-Name")); headerEvent != "" && headerEvent != eventName {
			return fiber.NewError(fiber.StatusBadRequest, "event mismatch")
		}

		switch eventName {
		case "subscription_created", "subscription_updated":
			var res lemonsqueezy.Resource[lemonsqueezy.SubscriptionAttributes]
			if err := json.Unmarshal(env.Data, &res); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid subscription data")
			}
			if h.storeID != 0 && res.Attributes.StoreID != h.storeID {
				return c.SendStatus(fiber.StatusOK)
			}
			lemonCustomerID := strconv.FormatInt(res.Attributes.CustomerID, 10)
			status := normalizeSubscriptionStatus(res.Attributes.Status)
			expiresAt := deriveExpiresAt(status, res.Attributes.RenewsAt, res.Attributes.EndsAt)
			_, err := h.customers.UpdateStatus(ctx, lemonCustomerID, status, expiresAt)
			if err != nil {
				return err
			}
			return c.SendStatus(fiber.StatusOK)

		case "license_key_created":
			var res lemonsqueezy.Resource[lemonsqueezy.LicenseKeyAttributes]
			if err := json.Unmarshal(env.Data, &res); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid license key data")
			}
			if h.storeID != 0 && res.Attributes.StoreID != h.storeID {
				return c.SendStatus(fiber.StatusOK)
			}
			lemonCustomerID := strconv.FormatInt(res.Attributes.CustomerID, 10)
			status := normalizeSubscriptionStatus(res.Attributes.Status)
			expiresAt := res.Attributes.ExpiresAt
			digest := h.customers.LicenseKeyDigest(res.Attributes.Key)
			if err := h.customers.UpsertLicenseKey(ctx, lemonCustomerID, digest, status, expiresAt); err != nil {
				return err
			}
			return c.SendStatus(fiber.StatusOK)

		default:
			return c.SendStatus(fiber.StatusOK)
		}
	}
}

func normalizeSubscriptionStatus(v string) domain.SubscriptionStatus {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "active":
		return domain.SubscriptionStatusActive
	case "past_due":
		return domain.SubscriptionStatusPastDue
	case "cancelled":
		return domain.SubscriptionStatusCancelled
	default:
		return domain.SubscriptionStatusCancelled
	}
}

func deriveExpiresAt(status domain.SubscriptionStatus, renewsAt *time.Time, endsAt *time.Time) *time.Time {
	switch status {
	case domain.SubscriptionStatusActive, domain.SubscriptionStatusPastDue:
		if renewsAt != nil {
			return renewsAt
		}
		return endsAt
	default:
		return endsAt
	}
}
