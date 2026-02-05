package http

import (
	"net/http"
	"testing"

	"github.com/velocli/velocli/velocli-backend/internal/config"
)

func TestSimulatorEndpoint_DisabledInProd(t *testing.T) {
	app := NewApp(config.Config{Env: "prod"}, Deps{})

	req, err := http.NewRequest(http.MethodPost, "/api/v1/test/simulate-purchase", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app test: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

