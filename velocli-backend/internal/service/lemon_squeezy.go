package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LemonSqueezyService struct {
	httpClient *http.Client
	apiKey     string
	storeID    int64
	productID  *int64
	variantID  *int64
}

type LicenseValidation struct {
	Valid      bool
	Status     string
	CustomerID int64
	ExpiresAt  *time.Time
}

func NewLemonSqueezyService(apiKey string, storeID int64, productID *int64, variantID *int64) *LemonSqueezyService {
	return &LemonSqueezyService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     strings.TrimSpace(apiKey),
		storeID:    storeID,
		productID:  productID,
		variantID:  variantID,
	}
}

func (s *LemonSqueezyService) ValidateLicense(key string) (bool, error) {
	res, err := s.ValidateLicenseDetailed(context.Background(), key)
	if err != nil {
		return false, err
	}
	return res.Valid, nil
}

func (s *LemonSqueezyService) ValidateLicenseDetailed(ctx context.Context, key string) (LicenseValidation, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return LicenseValidation{}, errors.New("license key is required")
	}

	form := url.Values{}
	form.Set("license_key", key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.lemonsqueezy.com/v1/licenses/validate", strings.NewReader(form.Encode()))
	if err != nil {
		return LicenseValidation{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return LicenseValidation{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return LicenseValidation{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LicenseValidation{}, errors.New(string(body))
	}

	var decoded struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
		LicenseKey struct {
			Status    string     `json:"status"`
			ExpiresAt *time.Time `json:"expires_at"`
		} `json:"license_key"`
		Meta struct {
			StoreID    int64 `json:"store_id"`
			ProductID  int64 `json:"product_id"`
			VariantID  int64 `json:"variant_id"`
			CustomerID int64 `json:"customer_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return LicenseValidation{}, err
	}
	if decoded.Error != "" {
		return LicenseValidation{}, errors.New(decoded.Error)
	}

	valid := decoded.Valid
	if s.storeID != 0 && decoded.Meta.StoreID != 0 && decoded.Meta.StoreID != s.storeID {
		valid = false
	}
	if s.productID != nil && decoded.Meta.ProductID != 0 && decoded.Meta.ProductID != *s.productID {
		valid = false
	}
	if s.variantID != nil && decoded.Meta.VariantID != 0 && decoded.Meta.VariantID != *s.variantID {
		valid = false
	}

	return LicenseValidation{
		Valid:      valid,
		Status:     decoded.LicenseKey.Status,
		CustomerID: decoded.Meta.CustomerID,
		ExpiresAt:  decoded.LicenseKey.ExpiresAt,
	}, nil
}
