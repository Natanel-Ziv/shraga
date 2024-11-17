package config

import (
	"shraga/internal/logging"

	"github.com/caarlos0/env/v8"
)

type Config struct {
    DSN string `env:"DATABASE_DSN" envDefault:"host=localhost user=postgres password=postgres dbname=monitoring port=5432 sslmode=disable"`
    Env string `env:"APP_ENV" envDefault:"dev"` // Environment type (e.g., prod, dev, test)
}

// LoadConfig loads configuration from environment variables or default values
func LoadConfig() Config {
    cfg := Config{}
    if err := env.Parse(&cfg); err != nil {
        logging.Logger.Sugar().Fatalf("Failed to load configuration: %v", err)
    }
    return cfg
}
