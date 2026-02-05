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
	"github.com/velocli/velocli/velocli-backend/internal/service"
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
	lemonService := service.NewLemonSqueezyService(cfg.LemonAPIKey, cfg.LemonStoreID, cfg.LemonProductID, cfg.LemonVariantID)
	jwtService := service.NewJWTService([]byte(cfg.JWTSigningKey), cfg.TokenTTL)

	authHandler := handlers.NewAuthHandler(lemonService, jwtService, customersRepo)
	bricksHandler := handlers.NewBricksHandler(customersRepo)
	lemonWebhook := handlers.NewLemonWebhookHandler(cfg.LemonWebhookSecret, cfg.LemonStoreID, cfg.VerifyLemonWebhookSignature, customersRepo)

	app := httpapi.NewApp(cfg, httpapi.Deps{
		LemonWebhook: lemonWebhook,
		Auth:         authHandler,
		Bricks:       bricksHandler,
		JWT:          jwtService,
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
