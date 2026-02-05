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
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	app := httpapi.NewApp(cfg)

	go func() {
		if err := app.Listen(cfg.HTTPAddr()); err != nil {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = app.ShutdownWithContext(ctx)
}
