package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env        string         `yaml:"env" env:"ENV" env-default:"development"`
	GRPCServer GRPCServer     `yaml:"grpc_server"`
	HTTPServer legacyTimeouts `yaml:"http_server"`
}

type GRPCServer struct {
	Port            int           `yaml:"port" env-default:"8085"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"15s"`
}

type legacyTimeouts struct {
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

func (c *Config) ServerShutdownTimeout() time.Duration {
	if c.GRPCServer.ShutdownTimeout > 0 {
		return c.GRPCServer.ShutdownTimeout
	}
	if c.HTTPServer.ShutdownTimeout > 0 {
		return c.HTTPServer.ShutdownTimeout
	}

	return 15 * time.Second
}

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
