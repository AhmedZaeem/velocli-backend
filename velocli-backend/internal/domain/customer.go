package domain

import "time"

type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusPastDue   SubscriptionStatus = "past_due"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
)

type Customer struct {
	ID                 string
	LemonCustomerID    string
	LicenseKeyDigest   string
	SubscriptionStatus SubscriptionStatus
	ExpiresAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
