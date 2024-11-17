package monitor

import (
	"context"
	"time"

	"gorm.io/gorm"
)

var now = time.Now

//go:generate stringer -type MonitorType -trimprefix Type
type MonitorType int

const (
	TypeUnknown MonitorType = iota
	TypeHTTP
)

//go:generate stringer -type Result -trimprefix Result
type Result int

const (
	ResultUnknown Result = iota
	ResultUp
	ResultDown
	ResultWarn
)

type MonitorResponser interface {
	GetBaseMonitorResponse() *BaseMonitorResponse
}

type BaseMonitorResponse struct {
	ID           uint `gorm:"primaryKey"`
	MonitorID    uint `gorm:"index"`
	ResponseTime time.Time
	Result       Result
	ErrorMsg     string
}

type Monitorer interface {
	Monitor(context.Context) MonitorResponser
	IsEnabled() bool
	GetType() MonitorType
	GetBase() *BaseMonitor
}

type BaseMonitor struct {
	ID              uint          `gorm:"primaryKey"`
	Type            MonitorType   `gorm:"index"`
	IntervalInt     int64         `gorm:"column:interval"` // Duration in nanoseconds
	Interval        time.Duration `gorm:"-"`
	Enabled         bool
	LastMonitorTime time.Time
	IsMonitoring    bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (b *BaseMonitor) BeforeSave(tx *gorm.DB) (err error) {
	// Serialize duration as nanoseconds
	b.IntervalInt = int64(b.Interval)
	return nil
}

func (b *BaseMonitor) AfterFind(tx *gorm.DB) (err error) {
	// Deserialize interval to time.Duration
	b.Interval = time.Duration(b.IntervalInt)
	return nil
}

func (b *BaseMonitor) GetBase() (*BaseMonitor) {
	return b
}

func (b *BaseMonitor) Monitor() MonitorResponser {
	panic("not implemented")
}

func (b *BaseMonitor) IsEnabled() bool {
	return b.Enabled
}

func (b *BaseMonitor) GetType() MonitorType {
	return b.Type
}
