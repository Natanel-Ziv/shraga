package db

import (
	"context"
	"fmt"
	"shraga/internal/logging"
	"shraga/internal/monitor"
	"time"

	"github.com/samber/lo"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

var now = time.Now

type GormDb struct {
	*gorm.DB
}

// NewGormDb returns new GormDb.
func NewGormDb(dsn string) (*GormDb, error) {
	logger := zapgorm2.New(logging.Logger)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{NowFunc: now, Logger: logger})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&monitor.HttpMonitor{}, &monitor.HttpResponse{})
	if err != nil {
		return nil, err
	}

	return &GormDb{db}, nil
}

func (db *GormDb) AddMonitor(ctx context.Context, monitor monitor.Monitorer) error {
	err := db.WithContext(ctx).Create(monitor).Error
	if err != nil {
		return err
	}
	return nil
}

func (db *GormDb) SaveResult(ctx context.Context, result monitor.MonitorResponser) error {
	err := db.WithContext(ctx).Create(result).Error
	if err != nil {
		return err
	}
	return nil
}

func (db *GormDb) GetEnabledMonitorsByType(ctx context.Context, monitorType monitor.MonitorType) ([]monitor.Monitorer, error) {
	var results []monitor.Monitorer

	switch monitorType {
	case monitor.TypeHTTP:
		var monitors []monitor.HttpMonitor
		if err := db.WithContext(ctx).Where("enabled = true").Find(&monitors).Error; err != nil {
			return nil, err
		}

		results = lo.Map(monitors, func(item monitor.HttpMonitor, _ int) monitor.Monitorer {
			return &item
		})
	case monitor.TypeUnknown:
		fallthrough
	default:
		return nil, fmt.Errorf("unknown type: %s", monitorType)
	}
	return results, nil
}

func (db *GormDb) GetMonitorsToRun(ctx context.Context) ([]monitor.Monitorer, error) {
	var results []monitor.Monitorer

	var monitors []monitor.HttpMonitor
	if err := db.WithContext(ctx).Where("enabled = true AND is_monitoring = false").Find(&monitors).Error; err != nil {
		return nil, err
	}

	nowTime := now()
	for _, mon := range monitors {
		if mon.LastMonitorTime.Add(mon.Interval).Before(nowTime) {
			results = append(results, &mon)
		}
	}

	return results, nil
}

func (db *GormDb) Lock(ctx context.Context, mon monitor.Monitorer) error {
	result := db.WithContext(ctx).
		Model(mon).
		Where("id = ?", mon.GetBase().ID).
		Update("is_monitoring", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("monitor with ID %d not found", mon.GetBase().ID)
	}
	return nil
}

func (db *GormDb) Unlock(ctx context.Context, mon monitor.Monitorer) error {
	result := db.WithContext(ctx).
		Model(mon).
		Where("id = ?", mon.GetBase().ID).
		Updates(map[string]any{
			"is_monitoring":     false,
			"last_monitor_time": now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("monitor with ID %d not found", mon.GetBase().ID)
	}
	return nil
}
