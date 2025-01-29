package config

import (
	"mycelium/internal/database"
	"time"
)

type Config struct {
	ListenAddr string
	Database   database.Config
	VerifyURL  string
	// Add more configuration options
}

func Load() (*Config, error) {
	// TODO: Implement config loading from file/env
	return &Config{
		ListenAddr: ":8080",
		Database: database.Config{
			Host:        "localhost",
			Port:        5432,
			User:        "postgres",
			Password:    "postgres",
			Database:    "mycelium",
			MaxConns:    10,
			MaxIdleTime: time.Minute * 3,
			HealthCheck: time.Second * 5,
			SSLMode:     "disable",
		},
		VerifyURL: "http://localhost:5000/verify", // Default verification endpoint
	}, nil
}
