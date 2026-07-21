package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                 string
	Environment          string
	DatabaseURL          string
	AllowedOrigin        string
	HTTPWriteTimeoutSec  int
	DatabasePoolMaxConns int
}

func Load() Config {
	return Config{
		Port:                 getEnv("PORT", "8080"),
		Environment:          getEnv("APP_ENV", "development"),
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		AllowedOrigin:        getEnv("ALLOWED_ORIGIN", "http://localhost:5173"),
		HTTPWriteTimeoutSec:  getEnvInt("HTTP_WRITE_TIMEOUT_SEC", 120),
		DatabasePoolMaxConns: getEnvInt("DATABASE_POOL_MAX_CONNS", 10),
	}
}

func getEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func getEnv(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	return value
}
