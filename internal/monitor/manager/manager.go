package manager

import (
	"context"
	"shraga/internal/db"
	"shraga/internal/logging"
	"shraga/internal/monitor"
	"sync"
	"time"

	"go.uber.org/zap"
)

const maxWorkers = 10

type Manager struct {
	db       db.Database
	doWorkCh chan monitor.Monitorer
	wg       *sync.WaitGroup
}

// NewManager returns new Manager.
func NewManager(db db.Database) *Manager {
	return &Manager{
		db:       db,
		doWorkCh: make(chan monitor.Monitorer),
		wg:       &sync.WaitGroup{},
	}
}

func (m *Manager) startWorkerPool(ctx context.Context) {
	logging.Logger.Sugar().Info("starting worker pool")
	for i := 0; i < maxWorkers; i++ {
		m.wg.Add(1)
		go func(workerId int) {
			logger := logging.Logger.Sugar().With("worker", workerId)
			logger.Info("started")
			defer m.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case mon, ok := <-m.doWorkCh:
					if !ok {
						logger.Info("channel closed, worker stopping")
						return
					}
					workLogger := logger.With("monitorID", mon.GetBase().ID)
					err := m.work(ctx, mon, workLogger)
					if err != nil {
						workLogger.Errorf("failed to monitor: %v", err)
					}
				}
			}
		}(i)
	}
}

func (m *Manager) Run(ctx context.Context) error {
	m.startWorkerPool(ctx)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Using a separate goroutine to close the channel
	go func() {
		<-ctx.Done()
		close(m.doWorkCh)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			availableMonitors, err := m.db.GetMonitorsToRun(ctx)
			if err != nil {
				logging.Logger.Sugar().Errorf("Failed to get monitors: %v", err)
				continue
			}

			for _, availableMonitor := range availableMonitors {
				select {
				case m.doWorkCh <- availableMonitor:
					// Successfully sent to worker
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

func (m *Manager) work(ctx context.Context, mon monitor.Monitorer, logger *zap.SugaredLogger) error {
	logger.Info("start monitoring")
	err := m.db.Lock(ctx, mon)
	if err != nil {
		return err
	}
	defer func() {
		unlockErr := m.db.Unlock(ctx, mon)
		if unlockErr != nil {
			logger.Errorf("failed to unlock monitor: %v", unlockErr)
		}
	}()

	result := mon.Monitor(ctx)
	err = m.db.SaveResult(ctx, result)
	if err != nil {
		return err
	}
	return nil

}
