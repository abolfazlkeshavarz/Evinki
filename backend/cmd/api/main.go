// Command api is the Evinki platform server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/bootstrap"
	"github.com/evinki/platform/internal/config"
	"github.com/evinki/platform/internal/platform/audit"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/payment"
	"github.com/evinki/platform/internal/platform/sms"
	"github.com/evinki/platform/internal/server"
	"github.com/evinki/platform/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DBURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if os.Getenv("MIGRATE_ON_START") != "false" {
		if err := pool.Migrate(ctx, migrations.FS, func(s string) { slog.Info("migration", "step", s) }); err != nil {
			return err
		}
	}

	var gw payment.Gateway = payment.Mock{PublicURL: cfg.PublicURL}
	if cfg.PaymentGateway == "zarinpal" {
		gw = payment.Zarinpal{Merchant: cfg.ZarinpalMerchant, Sandbox: cfg.ZarinpalSandbox}
	}
	d := &app.Deps{Cfg: cfg, DB: pool, Auth: auth.New(pool, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL),
		Audit: &audit.Logger{DB: pool}, Pay: gw}
	if err := bootstrap.Run(ctx, d); err != nil {
		return err
	}

	var sender sms.Sender = sms.Log{}
	if cfg.SMSEnabled {
		sender = sms.MeliPayamak{Username: cfg.SMSUsername, Password: cfg.SMSPassword}
	}
	go sms.Worker(ctx, pool, sender)

	srv := &http.Server{Addr: cfg.Addr, Handler: server.New(d), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "version", server.Version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
