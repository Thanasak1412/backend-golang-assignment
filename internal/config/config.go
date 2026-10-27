package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr    string
	MongoURI    string
	MongoDB     string
	JWTSecret   string
	JWTIssuer   string
	JWTDuration time.Duration
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load() (Config, error) {
	ttlHours, err := strconv.Atoi(value("JWT_TTL_HOURS", "1"))
	if err != nil || ttlHours <= 0 || ttlHours > 24 {
		return Config{}, errors.New("JWT_TTL_HOURS must be 1-24")
	}
	cfg := Config{
		HTTPAddr:    value("HTTP_ADDR", ":8080"),
		MongoURI:    value("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDB:     value("MONGODB_DATABASE", "user_management"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTIssuer:   value("JWT_ISSUER", "backend-golang-assignment"),
		JWTDuration: time.Duration(ttlHours) * time.Hour,
	}
	if len(cfg.JWTSecret) < 32 || cfg.JWTSecret == "REPLACE_WITH_RANDOM_64_HEX_CHAR_SECRET" {
		return Config{}, errors.New("JWT_SECRET must be at least 32 bytes")
	}
	return cfg, nil
}
