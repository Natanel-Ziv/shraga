package db

import (
	"context"
	"testing"
	"time"

	"shraga/internal/monitor"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type GormDbTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *GormDb
}

func (suite *GormDbTestSuite) SetupSuite() {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:13",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}
	var err error
	suite.container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	suite.Require().NoError(err)

	host, err := suite.container.Host(ctx)
	suite.Require().NoError(err)

	port, err := suite.container.MappedPort(ctx, "5432")
	suite.Require().NoError(err)

	dsn := "host=" + host + " port=" + port.Port() + " user=test password=test dbname=test sslmode=disable"
	suite.db, err = NewGormDb(dsn)
	suite.Require().NoError(err)

	err = suite.db.AutoMigrate(&monitor.HttpMonitor{}, &monitor.HttpResponse{})
	suite.Require().NoError(err)
}

func (suite *GormDbTestSuite) TearDownSuite() {
	suite.container.Terminate(context.Background())
}

func (suite *GormDbTestSuite) SetupTest() {
	err := suite.db.Exec("TRUNCATE TABLE http_monitors, http_responses RESTART IDENTITY CASCADE").Error
	suite.Require().NoError(err)
}

func (suite *GormDbTestSuite) TestAddMonitor() {
	mon := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:       1,
			Type:     monitor.TypeHTTP,
			Enabled:  true,
			Interval: time.Minute,
		},
		Address: "https://example.com",
	}

	err := suite.db.AddMonitor(context.Background(), mon)
	suite.NoError(err)

	var result monitor.HttpMonitor
	err = suite.db.First(&result, 1).Error
	suite.NoError(err)
	suite.Equal(mon.Address, result.Address)
}

func (suite *GormDbTestSuite) TestSaveResult() {

	result := &monitor.HttpResponse{
		BaseMonitorResponse: monitor.BaseMonitorResponse{
			ID:        1,
			MonitorID: 1,
			Result:    monitor.ResultUp,
		},
	}

	err := suite.db.SaveResult(context.Background(), result)
	suite.NoError(err)

	var savedResult monitor.HttpResponse
	err = suite.db.First(&savedResult, 1).Error
	suite.NoError(err)
	suite.Equal(result.Result, savedResult.Result)
}

func (suite *GormDbTestSuite) TestGetEnabledMonitorsByType() {

	mon := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:       1,
			Type:     monitor.TypeHTTP,
			Enabled:  true,
			Interval: time.Minute,
		},
		Address: "https://example.com",
	}

	err := suite.db.AddMonitor(context.Background(), mon)
	suite.NoError(err)

	monitors, err := suite.db.GetEnabledMonitorsByType(context.Background(), monitor.TypeHTTP)
	suite.NoError(err)
	suite.Len(monitors, 1)
	suite.Equal(mon.Address, monitors[0].(*monitor.HttpMonitor).Address)
}

func (suite *GormDbTestSuite) TestLockUnlock() {

	mon := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:       1,
			Type:     monitor.TypeHTTP,
			Enabled:  true,
			Interval: time.Minute,
		},
		Address: "https://example.com",
	}

	err := suite.db.AddMonitor(context.Background(), mon)
	suite.NoError(err)

	err = suite.db.Lock(context.Background(), mon)
	suite.NoError(err)

	var lockedMonitor monitor.HttpMonitor
	err = suite.db.First(&lockedMonitor, 1).Error
	suite.NoError(err)
	suite.True(lockedMonitor.IsMonitoring)

	err = suite.db.Unlock(context.Background(), mon)
	suite.NoError(err)

	var unlockedMonitor monitor.HttpMonitor
	err = suite.db.First(&unlockedMonitor, 1).Error
	suite.NoError(err)
	suite.False(unlockedMonitor.IsMonitoring)
}

func (suite *GormDbTestSuite) TestGetMonitorsToRun() {
	mon1 := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:              1,
			Type:            monitor.TypeHTTP,
			Enabled:         true,
			Interval:        time.Minute,
			LastMonitorTime: time.Now().Add(-2 * time.Minute),
		},
		Address: "https://example.com",
	}

	mon2 := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:              2,
			Type:            monitor.TypeHTTP,
			Enabled:         true,
			Interval:        5 * time.Minute,
			LastMonitorTime: time.Now().Add(-10 * time.Minute),
		},
		Address: "https://example2.com",
	}

	err := suite.db.AddMonitor(context.Background(), mon1)
	suite.NoError(err)

	err = suite.db.AddMonitor(context.Background(), mon2)
	suite.NoError(err)

	monitors, err := suite.db.GetMonitorsToRun(context.Background())
	suite.NoError(err)
	suite.Len(monitors, 2)
	suite.Equal(mon1.ID, monitors[0].GetBase().ID)
	suite.Equal(mon2.ID, monitors[1].GetBase().ID)
}

func (suite *GormDbTestSuite) TestGetEnabledMonitorsByType_UnknownType() {


	_, err := suite.db.GetEnabledMonitorsByType(context.Background(), monitor.TypeUnknown)
	suite.Error(err)
	suite.Equal("unknown type: Unknown", err.Error())
}

func (suite *GormDbTestSuite) TestLock_Error() {

	mon := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:       1,
			Type:     monitor.TypeHTTP,
			Enabled:  true,
			Interval: time.Minute,
		},
		Address: "https://example.com",
	}

	err := suite.db.Lock(context.Background(), mon)
	suite.Error(err)
}

func (suite *GormDbTestSuite) TestUnlock_Error() {
	mon := &monitor.HttpMonitor{
		BaseMonitor: monitor.BaseMonitor{
			ID:       999, // Use an ID that does not exist in the database
			Type:     monitor.TypeHTTP,
			Enabled:  true,
			Interval: time.Minute,
		},
		Address: "https://example.com",
	}

	// Attempt to unlock the monitor
	err := suite.db.Unlock(context.Background(), mon)
	suite.Error(err)
}

func TestGormDbTestSuite(t *testing.T) {
	suite.Run(t, new(GormDbTestSuite))
}
