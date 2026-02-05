package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/velocli/velocli/velocli-backend/internal/config"
	httpapi "github.com/velocli/velocli/velocli-backend/internal/http"
	"github.com/velocli/velocli/velocli-backend/internal/http/handlers"
	"github.com/velocli/velocli/velocli-backend/internal/platform/postgres"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	customersRepo := repository.NewCustomersRepository(pool, []byte(cfg.JWTSigningKey))
	lemonWebhook := handlers.NewLemonWebhookHandler(cfg.LemonWebhookSecret, cfg.LemonStoreID, customersRepo)

	app := httpapi.NewApp(cfg, httpapi.Deps{
		LemonWebhook: lemonWebhook,
	})

	go func() {
		if err := app.Listen(cfg.HTTPAddr()); err != nil {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	_ = app.ShutdownWithContext(shutdownCtx)
}
