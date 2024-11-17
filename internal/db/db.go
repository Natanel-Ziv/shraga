package db

import (
	"context"
	"shraga/internal/monitor"
)

type Database interface {
	AddMonitor(context.Context, monitor.Monitorer) error
	Lock(context.Context, monitor.Monitorer) error
	Unlock(context.Context, monitor.Monitorer) error
	SaveResult(ctx context.Context, result monitor.MonitorResponser) error
	GetEnabledMonitorsByType(context.Context, monitor.MonitorType) ([]monitor.Monitorer, error)
	GetMonitorsToRun(ctx context.Context) ([]monitor.Monitorer, error)
}
