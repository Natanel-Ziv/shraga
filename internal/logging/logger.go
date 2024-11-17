package logging

import (
	"sync"

	"github.com/samber/lo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
	once sync.Once
)

func init() {
	Logger = zap.L()
}

func Initialize(isProduction bool) {
	once.Do(func() {
		var cfg zap.Config
		if isProduction {
			cfg = zap.NewProductionConfig()
		} else {
			cfg = zap.NewDevelopmentConfig()
			cfg.EncoderConfig.TimeKey = "timestamp"
			cfg.EncoderConfig.LevelKey = "level"
			cfg.EncoderConfig.MessageKey = "msg"
			cfg.EncoderConfig.CallerKey = "caller"
			cfg.EncoderConfig.StacktraceKey = "stacktrace"
			cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		}

		Logger = lo.Must(cfg.Build())
	})
}
