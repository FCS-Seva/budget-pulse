package config

import "os"

type Config struct {
	HTTPAddr    string
	PostgresURL string
	NATSURL     string
}

func Load() Config {
	return Config{
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		PostgresURL: getEnv("POSTGRES_URL", "postgres://budgetpulse:budgetpulse@localhost:5432/budgetpulse?sslmode=disable"),
		NATSURL:     getEnv("NATS_URL", "nats://localhost:4222"),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
