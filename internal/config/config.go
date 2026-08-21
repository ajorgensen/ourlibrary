package config

import (
	"os"
	"time"
)

type Config struct {
	HTTPAddress       string
	DatabaseURL       string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

func Load() Config {
	return Config{
		HTTPAddress:       envOrDefault("HTTP_ADDRESS", ":8080"),
		DatabaseURL:       envOrDefault("DATABASE_URL", "postgres://ourlibrary:ourlibrary@localhost:5432/ourlibrary?sslmode=disable"),
		ReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:   10 * time.Second,
	}
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
