package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Login(ctx context.Context, licenseKey string) (string, error) {
	licenseKey = strings.TrimSpace(licenseKey)
	if licenseKey == "" {
		return "", errors.New("license key is required")
	}

	payload, _ := json.Marshal(map[string]string{"license_key": licenseKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("login failed")
	}

	var decoded struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", err
	}
	if strings.TrimSpace(decoded.Token) == "" {
		return "", errors.New("missing token")
	}
	return decoded.Token, nil
}

