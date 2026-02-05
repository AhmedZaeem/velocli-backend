package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/velocli/velocli/velocli-backend/internal/config"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
	"github.com/velocli/velocli/velocli-backend/internal/platform/postgres"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
)

func TestSimulatePurchase_Dev_UpsertsCustomer(t *testing.T) {
	port := freePort(t)
	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(port).
		Database("velocli").
		Username("postgres").
		Password("postgres"),
	)
	if err := db.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Stop() })

	databaseURL := "postgres://postgres:postgres@localhost:" + portString(port) + "/velocli?sslmode=disable"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	applySQL(t, pool, migrationPath(t, "0001_create_customers.sql"))
	applySQL(t, pool, migrationPath(t, "0002_create_bricks.sql"))

	customersRepo := repository.NewCustomersRepository(pool, []byte("test-hmac-key"))

	app := NewApp(config.Config{Env: "dev"}, Deps{
		TestSimulator: handlers.NewTestSimulatorHandler(customersRepo),
	})

	req, err := http.NewRequest(http.MethodPost, "/api/v1/test/simulate-purchase", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out struct {
		LemonCustomerID string `json:"lemon_customer_id"`
		LicenseKey      string `json:"license_key"`
		Status          string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.LemonCustomerID != "test@example.com" {
		t.Fatalf("unexpected lemon_customer_id: %q", out.LemonCustomerID)
	}
	if out.LicenseKey != "VELO-TEST-KEY-123" {
		t.Fatalf("unexpected license_key: %q", out.LicenseKey)
	}
	if out.Status != "active" {
		t.Fatalf("unexpected status: %q", out.Status)
	}

	saved, err := customersRepo.GetByLemonCustomerID(context.Background(), "test@example.com")
	if err != nil {
		t.Fatalf("get customer: %v", err)
	}
	if saved == nil {
		t.Fatalf("expected customer row to exist")
	}
	if saved.SubscriptionStatus != "active" {
		t.Fatalf("expected active status, got %s", saved.SubscriptionStatus)
	}
}

func freePort(t *testing.T) uint32 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	p, err := strconv.ParseUint(port, 10, 32)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return uint32(p)
}

func portString(p uint32) string {
	return strconv.FormatUint(uint64(p), 10)
}

func migrationPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations", name)
}

func applySQL(t *testing.T, pool *pgxpool.Pool, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := pool.Exec(context.Background(), string(b)); err != nil {
		t.Fatalf("exec %s: %v", path, err)
	}
}
