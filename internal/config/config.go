package config

import (
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env        string `yaml:"env" env:"ENV" env-default:"development"`
	GRPCServer `yaml:"grpc_server"`
}

type GRPCServer struct {
	Port int `yaml:"port" env-default:"8085"`
	// Address         string        `yaml:"address" env-default:"localhost:8080"`
	// ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"15s"`
	// CloseTimeout    time.Duration `yaml:"close_timeout" env-default:"10s"`
	// Timeout         time.Duration `yaml:"timeout" env-default:"4s"`
	// IdleTimeout     time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

// type Secrets struct {
// 	JWTSecret   string `env:"JWT_SECRET_KEY" env-required:"true"`
// 	PostgresURL string `env:"POSTGRES_URL" env-required:"true"`
// }

// func MustLoadSecrets() *Secrets {
// 	if err := godotenv.Load(); err != nil {
// 		log.Fatal(".env file is not opened")
// 	}

// 	var secrets Secrets

// 	secrets.JWTSecret = os.Getenv("JWT_SECRET_KEY")
// 	secrets.PostgresURL = os.Getenv("POSTGRES_URL")

// 	return &secrets
// }

func MustLoad() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file doesn't exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", err)
	}

	return &cfg
}
