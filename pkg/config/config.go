package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                 string
	Environment          string
	DatabaseURL          string
	AllowedOrigin        string
	JWTSecret            string
	GoogleClientID       string
	KrogerClientID       string
	KrogerClientSecret   string
	KrogerAPIBaseURL     string
	KrogerDefaultZip     string
	HTTPWriteTimeoutSec  int
	DatabasePoolMaxConns int
}

func Load() Config {
	return Config{
		Port:                 getEnv("PORT", "8080"),
		Environment:          getEnv("APP_ENV", "development"),
		DatabaseURL:          resolveDatabaseURL(),
		AllowedOrigin:        getEnv("ALLOWED_ORIGIN", "*"),
		JWTSecret:            getEnv("JWT_SECRET", ""),
		GoogleClientID:       getEnv("GOOGLE_CLIENT_ID", ""),
		KrogerClientID:       getEnv("KROGER_CLIENT_ID", ""),
		KrogerClientSecret:   getEnv("KROGER_CLIENT_SECRET", ""),
		KrogerAPIBaseURL:     getEnv("KROGER_API_BASE_URL", "https://api-ce.kroger.com/v1"),
		KrogerDefaultZip:     getEnv("KROGER_DEFAULT_ZIP", "45202"),
		HTTPWriteTimeoutSec:  getEnvInt("HTTP_WRITE_TIMEOUT_SEC", 120),
		DatabasePoolMaxConns: getEnvInt("DATABASE_POOL_MAX_CONNS", 5),
	}
}

// resolveDatabaseURL prefers DATABASE_URL, then common Neon/Vercel names.
func resolveDatabaseURL() string {
	candidates := []string{
		"DATABASE_URL",
		"POSTGRES_URL_NON_POOLING",
		"POSTGRES_URL",
		"POSTGRES_PRISMA_URL",
	}
	for _, key := range candidates {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
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
