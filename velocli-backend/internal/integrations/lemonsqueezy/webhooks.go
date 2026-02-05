package lemonsqueezy

import (
	"encoding/json"
	"time"
)

type WebhookMeta struct {
	EventName  string                 `json:"event_name"`
	CustomData map[string]any         `json:"custom_data,omitempty"`
}

type WebhookEnvelope struct {
	Meta WebhookMeta      `json:"meta"`
	Data json.RawMessage  `json:"data"`
}

type Resource[T any] struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes T      `json:"attributes"`
}

type SubscriptionAttributes struct {
	StoreID    int64      `json:"store_id"`
	CustomerID int64      `json:"customer_id"`
	Status     string     `json:"status"`
	RenewsAt   *time.Time `json:"renews_at"`
	EndsAt     *time.Time `json:"ends_at"`
	TestMode   bool       `json:"test_mode"`
}

type LicenseKeyAttributes struct {
	StoreID    int64      `json:"store_id"`
	CustomerID int64      `json:"customer_id"`
	Status     string     `json:"status"`
	Key        string     `json:"key"`
	ExpiresAt  *time.Time `json:"expires_at"`
	TestMode   bool       `json:"test_mode"`
}
