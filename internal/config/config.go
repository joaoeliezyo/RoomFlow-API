package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        int
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{Port: 8080, DatabaseURL: os.Getenv("DATABASE_URL")}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if value := os.Getenv("APP_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("invalid APP_PORT %q: expected a number from 1 to 65535", value)
		}
		cfg.Port = port
	}
	return cfg, nil
}
