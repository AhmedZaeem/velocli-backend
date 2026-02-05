package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
)

type CustomersRepository struct {
	pool   *pgxpool.Pool
	hmacKey []byte
}

func NewCustomersRepository(pool *pgxpool.Pool, hmacKey []byte) *CustomersRepository {
	keyCopy := make([]byte, len(hmacKey))
	copy(keyCopy, hmacKey)
	return &CustomersRepository{
		pool:   pool,
		hmacKey: keyCopy,
	}
}

func (r *CustomersRepository) LicenseKeyDigest(licenseKey string) string {
	mac := hmac.New(sha256.New, r.hmacKey)
	_, _ = mac.Write([]byte(licenseKey))
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum)
}

func (r *CustomersRepository) UpsertLicenseKey(ctx context.Context, lemonCustomerID string, licenseKeyDigest string, status domain.SubscriptionStatus, expiresAt *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO customers (lemon_customer_id, license_key, subscription_status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (lemon_customer_id) DO UPDATE
		SET license_key = EXCLUDED.license_key,
		    subscription_status = EXCLUDED.subscription_status,
		    expires_at = EXCLUDED.expires_at,
		    updated_at = now()
	`, lemonCustomerID, licenseKeyDigest, status, expiresAt)
	return err
}

func (r *CustomersRepository) UpdateStatus(ctx context.Context, lemonCustomerID string, status domain.SubscriptionStatus, expiresAt *time.Time) (bool, error) {
	var updated int64
	err := r.pool.QueryRow(ctx, `
		UPDATE customers
		SET subscription_status = $2,
		    expires_at = $3,
		    updated_at = now()
		WHERE lemon_customer_id = $1
		RETURNING 1
	`, lemonCustomerID, status, expiresAt).Scan(&updated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *CustomersRepository) GetByLemonCustomerID(ctx context.Context, lemonCustomerID string) (*domain.Customer, error) {
	var c domain.Customer
	err := r.pool.QueryRow(ctx, `
		SELECT id, lemon_customer_id, license_key, subscription_status, expires_at, created_at, updated_at
		FROM customers
		WHERE lemon_customer_id = $1
	`, lemonCustomerID).Scan(
		&c.ID,
		&c.LemonCustomerID,
		&c.LicenseKeyDigest,
		&c.SubscriptionStatus,
		&c.ExpiresAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
