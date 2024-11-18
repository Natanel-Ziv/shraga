package main

import (
	"context"
	"os"
	"os/signal"
	"shraga/internal/config"
	"shraga/internal/db"
	"shraga/internal/logging"
	"shraga/internal/monitor/manager"
	"syscall"

	"github.com/samber/lo"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelCtx()

	cfg := config.LoadConfig()

	logging.Initialize(cfg.Env == "prod")
	logging.Logger.Info("Logger initialized")
	defer logging.Logger.Sync()

	gormDB := lo.Must(db.NewGormDb(cfg.DSN))

	monitorMgr := manager.NewManager(gormDB)
	go monitorMgr.Run(ctx)
	<-ctx.Done()
	logging.Logger.Info("exiting")
}
